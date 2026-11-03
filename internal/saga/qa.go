package saga

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The QA records say where a repository's end-to-end tests live and how to run a
// chosen few. A test case is in the catalog because its code is in one of these
// paths, so nothing is marked on the test case. A Saga without them is unaffected:
// some apps have no end-to-end tests and are fine.
//
//	___qa/
//	  suite.json              repository paths, command template
const (
	QADir         = "___qa"
	QASuiteName   = "suite.json"
	QASuiteSchema = "https://changesaga.dev/schema/v5/qa-suite.schema.json"

	// QASuiteLocations is what a command template names the tests to run with.
	QASuiteLocations = "{locations}"
)

// QASuite is ___qa/suite.json. The tool never runs RunCommand.
type QASuite struct {
	Schema     string   `json:"$schema"`
	Version    int      `json:"version"`
	Paths      []string `json:"paths"`
	RunCommand string   `json:"run_command,omitempty"`
}

// QASuitePath is the app-relative path of the suite record.
func QASuitePath() string { return QADir + "/" + QASuiteName }

// ValidateQASuite checks suite.json on its own. Whether a path exists is reported
// when the suite is read: the tests may not have been written yet.
func ValidateQASuite(suite QASuite) error {
	switch {
	case suite.Schema != QASuiteSchema:
		return fmt.Errorf("$schema must be %s", QASuiteSchema)
	case suite.Version != SagaVersion:
		return fmt.Errorf("version must be %d", SagaVersion)
	case len(suite.Paths) == 0:
		return fmt.Errorf("at least one path is required")
	case suite.RunCommand != "" && !strings.Contains(suite.RunCommand, QASuiteLocations):
		return fmt.Errorf("the command must name the tests to run with %s", QASuiteLocations)
	}
	seen := map[string]bool{}
	for _, value := range suite.Paths {
		if value == "" || path.IsAbs(value) || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
			return fmt.Errorf("path %q must be a clean path inside the repository", value)
		}
		if seen[value] {
			return fmt.Errorf("path %q appears more than once", value)
		}
		seen[value] = true
	}
	return nil
}

func loadQASuite(root string, validation *Validation) *QASuite {
	dir := filepath.Join(root, QADir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		addIssue(validation, "error", QADir, err.Error())
		return nil
	}
	var suite *QASuite
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, ".change-saga-"):
		case name == QASuiteName && entry.Type().IsRegular():
			var value QASuite
			if err := readJSON(filepath.Join(dir, name), &value); err != nil {
				addIssue(validation, "error", QASuitePath(), err.Error())
				continue
			}
			if err := ValidateQASuite(value); err != nil {
				addIssue(validation, "error", QASuitePath(), err.Error())
			}
			suite = &value
		default:
			addIssue(validation, "error", relativePath(root, filepath.Join(dir, name)), "unknown entry in ___qa; it holds suite.json")
		}
	}
	return suite
}
