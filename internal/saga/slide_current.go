package saga

import (
	"encoding/json"
	"fmt"

	"github.com/twentyideas/changesaga/internal/currentrecord"
)

const SlideCurrentSchema = "https://changesaga.dev/schema/v5/slide-current.schema.json"

// SlideContentDigest pins the complete current slide without authoring or
// history metadata. Changing a request ID cannot manufacture a new version.
func SlideContentDigest(content SlideTransactionRevision) (string, error) {
	data, err := slideCurrentContent(content)
	if err != nil {
		return "", err
	}
	return currentrecord.Digest(data)
}

func slideCurrentContent(content SlideTransactionRevision) (json.RawMessage, error) {
	data, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for _, key := range []string{"snapshot", "parent_snapshots", "request_id", "created_at"} {
		delete(fields, key)
	}
	return json.Marshal(fields)
}

// MarshalJSON publishes exactly one editable slide. Legacy history is accepted
// only by the decoder; it cannot be emitted by any current writer.
func (record SlideTransactionRecord) MarshalJSON() ([]byte, error) {
	heads, err := record.Heads()
	if err != nil {
		return nil, err
	}
	if len(heads) != 1 {
		return nil, fmt.Errorf("legacy slide has competing heads %v; preserve the file in Git and combine its content explicitly before updating", heads)
	}
	content, err := record.currentRevision()
	if err != nil {
		return nil, err
	}
	data, err := slideCurrentContent(*content)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Schema  string          `json:"$schema"`
		Version int             `json:"version"`
		Deck    string          `json:"deck"`
		Slide   string          `json:"slide"`
		Content json.RawMessage `json:"content"`
	}{SlideCurrentSchema, 2, record.DeckID, record.SlideID, data})
}

func (record *SlideTransactionRecord) UnmarshalJSON(data []byte) error {
	var value struct {
		Schema    string                     `json:"$schema,omitempty"`
		Version   int                        `json:"version"`
		DeckID    string                     `json:"deck"`
		SlideID   string                     `json:"slide"`
		Content   json.RawMessage            `json:"content,omitempty"`
		Current   string                     `json:"current,omitempty"`
		Revisions []SlideTransactionRevision `json:"revisions,omitempty"`
	}
	if err := currentrecord.Decode(data, &value); err != nil {
		return err
	}
	if len(value.Content) == 0 {
		if value.Version != SlideTransactionVersion {
			return fmt.Errorf("unsupported legacy slide transaction version %d", value.Version)
		}
		*record = SlideTransactionRecord{Version: value.Version, DeckID: value.DeckID, SlideID: value.SlideID, Current: value.Current, Revisions: value.Revisions}
		return nil
	}
	if value.Schema != SlideCurrentSchema || value.Version != 2 || value.Current != "" || len(value.Revisions) != 0 {
		return fmt.Errorf("current slide requires schema/version 2 and one content value")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value.Content, &fields); err != nil {
		return err
	}
	for _, key := range []string{"snapshot", "parent_snapshots", "request_id", "created_at"} {
		if _, ok := fields[key]; ok {
			return fmt.Errorf("current slide cannot contain %s", key)
		}
	}
	var content SlideTransactionRevision
	if err := currentrecord.Decode(value.Content, &content); err != nil {
		return err
	}
	digest, err := SlideContentDigest(content)
	if err != nil {
		return err
	}
	content.Snapshot = digest
	*record = SlideTransactionRecord{Version: SlideTransactionVersion, DeckID: value.DeckID, SlideID: value.SlideID, Current: digest, Revisions: []SlideTransactionRevision{content}, canonical: true}
	return nil
}
