package requirements

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/currentrecord"
)

func projectCurrent(record *currentrecord.Record, createdAt time.Time, contentSchema, stateSchema string, version int, content, state any) error {
	digest, err := currentrecord.Digest(record.Content)
	if err != nil {
		return err
	}
	seed := func(schema, id string, raw json.RawMessage, target any) error {
		meta := map[string]any{"$schema": schema, "version": version, "id": id, "parents": []string{}, "created_at": createdAt}
		encoded, _ := json.Marshal(meta)
		if err := json.Unmarshal(encoded, target); err != nil {
			return err
		}
		return currentrecord.Project(raw, target)
	}
	if err := seed(contentSchema, "sha256-"+strings.TrimPrefix(digest, "sha256:"), record.Content, content); err != nil {
		return err
	}
	if len(record.State) == 0 {
		return fmt.Errorf("current documentation requires state")
	}
	return seed(stateSchema, "current", record.State, state)
}
