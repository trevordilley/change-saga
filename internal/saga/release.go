package saga

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A Release is a named version of the product and everything needed to get it
// to users. It belongs to no feature, and a Saga without any is unaffected. Git
// is the history of a release: when it changes it is the same record, committed
// again, and a release that should not exist is deleted.
//
//	___releases/<id>.release/
//	  release.json            version, range, milestone, released commit
const (
	ReleasesDir         = "___releases"
	ReleaseSuffix       = ".release"
	ReleaseManifestName = "release.json"

	ReleaseRecordVersion = SagaVersion
	ReleaseSchemaURL     = "https://changesaga.dev/schema/v5/release.schema.json"
)

// ReleaseManifest is release.json. Version is the record format, as on every
// record, so the release's own version is ReleaseVersion.
type ReleaseManifest struct {
	Schema  string `json:"$schema"`
	Version int    `json:"version"`
	ID      string `json:"id"`
	// ReleaseVersion is exactly what a person supplied. Nothing computes it, and
	// no scheme is enforced: it need not be a legal record ID.
	ReleaseVersion string `json:"release_version"`
	// From and To are the Git revisions the release runs between, as given. A To
	// naming a tag not yet pushed is valid, so they are not resolved here.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// Milestone is the URN of the shared milestone the release sits within.
	Milestone string `json:"milestone,omitempty"`
	// ReleasedAt and ReleasedCommit record that someone said it went out. The
	// commit may be earlier than the commit holding this record.
	ReleasedAt     *time.Time `json:"released_at,omitempty"`
	ReleasedCommit string     `json:"released_commit,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Release is one loaded release.
type Release struct {
	ReleaseManifest
	// Path is the app-relative release directory.
	Path      string `json:"path"`
	Directory string `json:"-"`
	Target    string `json:"target"`
}

// ReleaseTarget is the URN of a release.
func ReleaseTarget(sagaID, releaseID string) string {
	return "urn:change-saga:" + sagaID + ":release:" + releaseID
}

// ReleaseDir is the directory of a release.
func ReleaseDir(root, id string) string {
	return filepath.Join(root, ReleasesDir, id+ReleaseSuffix)
}

// FindRelease returns the release with this ID, or nil.
func (document *Saga) FindRelease(id string) *Release {
	for _, release := range document.Releases {
		if release.ID == id {
			return release
		}
	}
	return nil
}

// Released reports whether someone has marked the release as gone out.
func (release *Release) Released() bool { return release.ReleasedAt != nil }

var releaseCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// ValidateReleaseManifest checks one release.json on its own. Whether a milestone
// still exists, or a range resolves, is reported elsewhere rather than making an
// otherwise good Saga invalid.
func ValidateReleaseManifest(sagaID string, manifest ReleaseManifest) error {
	switch {
	case manifest.Schema != ReleaseSchemaURL:
		return fmt.Errorf("$schema must be %s", ReleaseSchemaURL)
	case manifest.Version != ReleaseRecordVersion:
		return fmt.Errorf("version must be %d", ReleaseRecordVersion)
	case !ValidID(manifest.ID):
		return fmt.Errorf("id must be a stable identifier")
	case manifest.ReleaseVersion == "" || strings.TrimSpace(manifest.ReleaseVersion) != manifest.ReleaseVersion || len(manifest.ReleaseVersion) > 200:
		return fmt.Errorf("release_version must hold 1 to 200 bytes without surrounding whitespace")
	case manifest.CreatedAt.IsZero():
		return fmt.Errorf("created_at is required")
	case manifest.From != strings.TrimSpace(manifest.From) || manifest.To != strings.TrimSpace(manifest.To):
		return fmt.Errorf("from and to must not have surrounding whitespace")
	case manifest.Milestone != "" && !validReleaseMilestone(sagaID, manifest.Milestone):
		return fmt.Errorf("milestone must be a milestone URN of this Saga")
	case (manifest.ReleasedAt == nil) != (manifest.ReleasedCommit == ""):
		return fmt.Errorf("released_at and released_commit are recorded together")
	case manifest.ReleasedAt != nil && manifest.ReleasedAt.IsZero():
		return fmt.Errorf("released_at must be a time when present")
	case manifest.ReleasedCommit != "" && !releaseCommitPattern.MatchString(manifest.ReleasedCommit):
		return fmt.Errorf("released_commit must be a full commit ID")
	}
	return nil
}

func validReleaseMilestone(sagaID, urn string) bool {
	id, found := strings.CutPrefix(urn, "urn:change-saga:"+sagaID+":milestone:")
	return found && ValidID(id)
}

// loadReleases loads every release beneath ___releases. Two releases of one
// version are an error, so branches that each create it cannot merge quietly.
func loadReleases(root string, manifest Manifest, validation *Validation) ([]*Release, error) {
	dir := filepath.Join(root, ReleasesDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var releases []*Release
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".change-saga-") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		id := strings.TrimSuffix(entry.Name(), ReleaseSuffix)
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(entry.Name(), ReleaseSuffix) || !ValidID(id) {
			addIssue(validation, "error", relativePath(root, path), "releases must be real <id>.release directories")
			continue
		}
		release, err := loadRelease(root, path, id, manifest, validation)
		if err != nil {
			return nil, err
		}
		if release != nil {
			releases = append(releases, release)
		}
	}
	sort.Slice(releases, func(i, j int) bool { return releases[i].ID < releases[j].ID })
	versions := map[string]string{}
	for _, release := range releases {
		if other, taken := versions[release.ReleaseVersion]; taken {
			addIssue(validation, "error", release.Path, fmt.Sprintf("version %q is already release %q", release.ReleaseVersion, other))
			continue
		}
		versions[release.ReleaseVersion] = release.ID
	}
	return releases, nil
}

func loadRelease(root, dir, id string, manifest Manifest, validation *Validation) (*Release, error) {
	manifestPath := filepath.Join(dir, ReleaseManifestName)
	var value ReleaseManifest
	if err := readJSON(manifestPath, &value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
		return nil, nil
	}
	if err := ValidateReleaseManifest(manifest.ID, value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
	}
	if value.ID != id {
		addIssue(validation, "error", relativePath(root, manifestPath), fmt.Sprintf("release id %q must match its directory %q", value.ID, id+ReleaseSuffix))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name() == ReleaseManifestName || strings.HasPrefix(entry.Name(), ".change-saga-") {
			continue
		}
		addIssue(validation, "error", relativePath(root, filepath.Join(dir, entry.Name())), "unknown entry in a release; a release holds release.json")
	}
	return &Release{ReleaseManifest: value, Path: relativePath(root, dir), Directory: dir, Target: ReleaseTarget(manifest.ID, id)}, nil
}
