package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/prototypes"
)

const (
	prototypeQueryDefaultBytes = 64 << 10
	prototypeQueryMaxBytes     = 256 << 10
)

type prototypeQueryRecord struct {
	ID          string                  `json:"id"`
	Target      string                  `json:"target"`
	Feature     string                  `json:"feature"`
	Title       string                  `json:"title"`
	State       string                  `json:"state"`
	Revision    string                  `json:"revision"`
	Current     bool                    `json:"current"`
	Revisions   []string                `json:"revisions"`
	Source      prototypes.Source       `json:"source"`
	Files       []string                `json:"files"`
	File        string                  `json:"file,omitempty"`
	Content     *prototypeQueryContent  `json:"content,omitempty"`
	Annotations []prototypes.Annotation `json:"annotations"`
}

// prototypeQueryContent is one bounded byte range of a prototype's file, so one
// large app cannot fill a session. NextOffset continues it.
type prototypeQueryContent struct {
	MediaType  string `json:"media_type"`
	Data       string `json:"data"`
	Offset     int64  `json:"offset"`
	TotalBytes int64  `json:"total_bytes"`
	NextOffset *int64 `json:"next_offset"`
}

// queryPrototype reads a prototype's HTML as it was recorded, from the Saga for
// a copied revision or from its pinned commit for one that lives in the project.
// One hosted elsewhere has no source to read: its URLs are returned instead.
func queryPrototype(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query prototype", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "Saga root")
	selector := flags.String("prototype", "", "prototype ID or URN")
	revisionID := flags.String("revision", "", "revision ID; defaults to the current one")
	file := flags.String("file", "", "file within the prototype; defaults to its entrypoint")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a separate repository")
	offset := flags.Int64("offset", 0, "content byte offset")
	limit := flags.Int64("limit", prototypeQueryDefaultBytes, "content bytes, 1 to 262144")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("prototype"), nil)
		}
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: err.Error()})
	}
	if *root == "" || *selector == "" || flags.NArg() != 0 || *offset < 0 || *limit < 1 || *limit > prototypeQueryMaxBytes {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: queryUsage["prototype"]})
	}
	document, err := prototypes.Load(*root, "")
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	id := *selector
	if strings.HasPrefix(id, "urn:") {
		var ok bool
		if id, ok = strings.CutPrefix(id, "urn:change-saga:"+document.SagaID+":prototype:"); !ok {
			return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "--prototype must be a prototype ID or URN of this Saga"})
		}
	}
	prototype := document.Find(id)
	if prototype == nil {
		return writeQueryFailure(out, &queryError{Code: "not_found", Message: "prototype does not exist"})
	}
	currentID := ""
	if prototype.CurrentRevision != nil {
		currentID = prototype.CurrentRevision.ID
	}
	wanted := *revisionID
	if wanted == "" {
		wanted = currentID
	}
	if wanted == "" {
		return writeQueryFailure(out, &queryError{Code: "conflict", Message: "prototype has several current revisions; pass --revision", Details: map[string]any{"heads": prototype.RevisionHeads}})
	}
	revision := prototype.Revision(wanted)
	if revision == nil {
		return writeQueryFailure(out, &queryError{Code: "not_found", Message: "prototype revision does not exist"})
	}
	target, _ := prototypes.PrototypeURN(document.SagaID, id)
	record := prototypeQueryRecord{
		ID: id, Target: target, Feature: prototype.Feature, Title: revision.Title, State: string(revision.State),
		Revision: revision.ID, Current: revision.ID == currentID, Source: revision.Source,
		Revisions: []string{}, Files: []string{}, Annotations: []prototypes.Annotation{},
	}
	for _, value := range prototype.Revisions {
		record.Revisions = append(record.Revisions, value.ID)
	}
	for _, annotation := range document.Annotations {
		if annotation.Prototype == target {
			record.Annotations = append(record.Annotations, annotation)
		}
	}
	var data []byte
	switch revision.Source.Kind {
	case prototypes.SourceHTML:
		dir, err := document.RevisionDir(*prototype, revision.ID)
		if err != nil {
			return writeQueryFailure(out, normalizeQueryError(err))
		}
		record.Files, err = prototypePackageFiles(filepath.Join(dir, "html"))
		if err != nil {
			return writeQueryFailure(out, normalizeQueryError(err))
		}
		record.File = strings.TrimPrefix(revision.Source.Entrypoint, "html/")
		if *file != "" {
			record.File = path.Clean("/" + filepath.ToSlash(*file))[1:]
		}
		if !slices.Contains(record.Files, record.File) {
			return writeQueryFailure(out, &queryError{Code: "not_found", Message: "the prototype has no such file", Details: map[string]any{"files": record.Files}})
		}
		if data, err = os.ReadFile(filepath.Join(dir, "html", filepath.FromSlash(record.File))); err != nil {
			return writeQueryFailure(out, normalizeQueryError(err))
		}
	case prototypes.SourceRepo:
		checkout := *repo
		if checkout == "" {
			checkout = *root
		}
		record.Files = repoPrototypeFiles(ctx, checkout, revision.Source)
		record.File = revision.Source.RepoEntry()
		if *file != "" {
			record.File = path.Clean("/" + filepath.ToSlash(*file))[1:]
		}
		var ok bool
		if data, ok = prototypes.ReadRepoFile(ctx, checkout, revision.Source, record.File); !ok {
			return writeQueryFailure(out, &queryError{Code: "not_found", Message: "the file cannot be read at the prototype's pinned commit", Details: map[string]any{"file": record.File, "commit": revision.Source.Commit}})
		}
	default:
		if *file != "" {
			return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "this prototype is hosted elsewhere and has no files; its source names where"})
		}
	}
	if record.File != "" {
		record.Content = prototypeContent(record.File, data, *offset, *limit)
	}
	encoded, _ := json.Marshal(revision)
	snapshot := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
	return writeQuerySuccess(out, snapshot, map[string]any{"prototype": record}, nil)
}

// prototypeContent cuts one range on a character boundary, so each page is valid
// text and pages join back into the file.
func prototypeContent(name string, data []byte, offset, limit int64) *prototypeQueryContent {
	total := int64(len(data))
	content := &prototypeQueryContent{MediaType: mime.TypeByExtension(strings.ToLower(path.Ext(name))), Offset: min(offset, total), TotalBytes: total}
	end := min(content.Offset+limit, total)
	for end > content.Offset && end < total && !utf8.RuneStart(data[end]) {
		end--
	}
	content.Data = string(data[content.Offset:end])
	if end < total {
		content.NextOffset = &end
	}
	return content
}

func prototypePackageFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, current)
		if err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return err
	})
	return files, err
}

func repoPrototypeFiles(ctx context.Context, checkout string, source prototypes.Source) []string {
	if source.Root == "" {
		return []string{source.RepoEntry()}
	}
	listed, err := gitexec.Output(ctx, "-C", checkout, "ls-tree", "-r", "--name-only", source.Commit, "--", source.Root+"/")
	files := []string{}
	if err != nil {
		return files
	}
	for _, line := range strings.Split(strings.TrimSpace(string(listed)), "\n") {
		if rel, ok := strings.CutPrefix(line, source.Root+"/"); ok {
			files = append(files, rel)
		}
	}
	return files
}
