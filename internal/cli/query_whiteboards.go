package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/twentyideas/changesaga/internal/saga"
)

func queryWhiteboards(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query whiteboards", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "Saga root")
	cursor := flags.String("cursor", "", "snapshot-bound cursor")
	limit := flags.Int("limit", 50, "page size, 1..1000")
	fail := func(code string, err error) error {
		return writeQueryFailure(out, &queryError{Code: code, Message: err.Error()})
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("whiteboards"), nil)
		}
		return fail("invalid_argument", err)
	}
	if *root == "" || flags.NArg() != 0 || len(*cursor) > 4096 || *limit < 1 || *limit > maxQueryPageSize {
		return fail("invalid_argument", fmt.Errorf("requires --saga, --limit 1..1000 and a cursor of at most 4096 bytes"))
	}
	manifest, err := saga.ReadManifest(*root)
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	if manifest.Version != 5 {
		return fail("invalid_saga", fmt.Errorf("whiteboards require a v5 Saga"))
	}
	result, err := loadWhiteboardList(*root, manifest.ID)
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	encoded, _ := json.Marshal(result)
	digest := sha256.Sum256(encoded)
	snapshot := hex.EncodeToString(digest[:])
	absolute, err := filepath.Abs(*root)
	if err != nil {
		return fail("invalid_argument", err)
	}
	key := "whiteboards:" + absolute + ":" + manifest.ID
	offset, cursorErr := decodeTermsCursor(*cursor, key, snapshot, len(result.Whiteboards))
	if cursorErr != nil {
		return writeQueryFailure(out, cursorErr)
	}
	total := len(result.Whiteboards)
	end := min(total, offset+*limit)
	page := queryPageEnvelope{Total: total, Returned: end - offset, HasMore: end < total}
	if page.HasMore {
		next := encodeTermsCursor(key, snapshot, end)
		page.NextCursor = &next
	}
	result.Whiteboards = result.Whiteboards[offset:end]
	return writeQuerySuccess(out, snapshot, result, &page)
}
