package requirements

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// DecodeInventoryJSON enforces the closed, bounded technical-record contract.
// Duplicate keys must not let two readers disagree about a pin or code digest.
func DecodeInventoryJSON(data []byte, value any) error {
	if len(data) > MaxRecordBytes {
		return fmt.Errorf("technical record exceeds one MiB")
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := tokens.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for tokens.More() {
				key, err := tokens.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok {
					return fmt.Errorf("invalid object key")
				}
				if seen[name] {
					return fmt.Errorf("duplicate JSON key %q", name)
				}
				seen[name] = true
				if err := walk(); err != nil {
					return err
				}
			}
		case '[':
			for tokens.More() {
				if err := walk(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected delimiter")
		}
		_, err = tokens.Token()
		return err
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return fmt.Errorf("technical record must contain exactly one JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func readInventoryJSON(path string, value any) error {
	// The regular-file/symlink and size checks remain the common loader's.
	if err := readStrictJSON(path, value); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return DecodeInventoryJSON(data, value)
}
