package cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"

	"github.com/twentyideas/changesaga/internal/saga"
)

type doctorBuild struct {
	Version            string `json:"version"`
	Commit             string `json:"commit,omitempty"`
	BuildDate          string `json:"build_date,omitempty"`
	Executable         string `json:"executable"`
	ResolvedExecutable string `json:"resolved_executable"`
	SHA256             string `json:"sha256,omitempty"`
	ExecutableError    string `json:"executable_error,omitempty"`
	GoVersion          string `json:"go_version"`
	VCSRevision        string `json:"vcs_revision,omitempty"`
	VCSModified        *bool  `json:"vcs_modified"`
}

type doctorCapabilities struct {
	Plans struct {
		Layout      string   `json:"layout"`
		Authoring   bool     `json:"authoring"`
		Queries     []string `json:"queries"`
		Checkpoints bool     `json:"checkpoints"`
		LegacyWaves bool     `json:"legacy_waves"`
	} `json:"plans"`
	Saga struct {
		ManifestVersion int  `json:"manifest_version"`
		CurrentRecords  bool `json:"current_records"`
		LegacyReads     bool `json:"legacy_reads"`
		StrictJSON      bool `json:"strict_json"`
	} `json:"saga"`
}

type doctorSagaCheck struct {
	Path string `json:"path"`
	saga.Validation
	Error string `json:"error,omitempty"`
}

type doctorReport struct {
	Schema       string             `json:"schema"`
	Build        doctorBuild        `json:"build"`
	Capabilities doctorCapabilities `json:"capabilities"`
	SagaCheck    *doctorSagaCheck   `json:"saga_check,omitempty"`
	Guidance     []string           `json:"guidance"`
}

// Doctor describes the running build without executing other PATH candidates,
// calling a service, or writing Saga data. Capabilities describe this build;
// only a full validation can assess the supplied Saga.
func Doctor(_ context.Context, args []string, out io.Writer) error {
	flags := commandFlags("doctor", commandUsage["doctor"], out)
	jsonOutput := flags.Bool("json", false, "emit machine-readable diagnostics")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("usage: %s", commandUsage["doctor"])
	}
	report := doctorReport{Schema: "change-saga.doctor/v1", Build: runningDoctorBuild(), Guidance: []string{
		"Check shell resolution with type -a change-saga; use the same absolute executable for authoring, queries and serving.",
		"Phased Plan authoring, Plan queries, checkpoint fields and whole-Saga layouts are separate capabilities.",
		"v0.8.0 supports phased Plans and Plan queries, but rejects normalized current-record Sagas; manifest version 5 alone is insufficient.",
		"This build supports native checkpoints and current records together. Legacy waves and unknown fields are rejected; never delete fields to make a reader accept the data.",
		"Validate the actual whole Saga with the pinned build. See docs/plan-cli-compatibility.md; no normalization or evidence repinning is performed here.",
	}}
	report.Capabilities.Plans.Layout = "phases"
	report.Capabilities.Plans.Authoring = true
	report.Capabilities.Plans.Checkpoints = true
	report.Capabilities.Plans.Queries = []string{"plans", "plan", "task"}
	report.Capabilities.Saga.ManifestVersion = saga.SagaVersion
	report.Capabilities.Saga.CurrentRecords = true
	report.Capabilities.Saga.LegacyReads = true
	report.Capabilities.Saga.StrictJSON = true
	if flags.NArg() == 1 {
		root := flags.Arg(0)
		_, validation, err := validateSaga(root)
		report.SagaCheck = &doctorSagaCheck{Path: root, Validation: validation}
		if err != nil {
			report.SagaCheck.Valid = false
			report.SagaCheck.Error = err.Error()
		}
	}
	if *jsonOutput {
		if err := writeJSON(out, report); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "Build: %s\nInjected commit: %s\nExecutable: %s\nResolved executable: %s\nSHA256: %s\nGo: %s\n", report.Build.Version, report.Build.Commit, report.Build.Executable, report.Build.ResolvedExecutable, report.Build.SHA256, report.Build.GoVersion)
		fmt.Fprintf(out, "VCS revision: %s; modified: %v (null means unavailable)\n", report.Build.VCSRevision, doctorModifiedText(report.Build.VCSModified))
		if report.Build.ExecutableError != "" {
			fmt.Fprintf(out, "Executable inspection: %s\n", report.Build.ExecutableError)
		}
		fmt.Fprintln(out, "Plans: phases; authoring: yes; queries: plans, plan, task; checkpoints: yes; legacy waves: no")
		fmt.Fprintf(out, "Saga: manifest v%d; current records and legacy reads; strict JSON decoding\n", saga.SagaVersion)
		if check := report.SagaCheck; check != nil {
			fmt.Fprintf(out, "Whole-Saga validation: %s; valid: %t\n", check.Path, check.Valid)
			if check.Error != "" {
				fmt.Fprintln(out, "  "+check.Error)
			}
			for _, issue := range check.Issues {
				fmt.Fprintf(out, "  %s: %s: %s\n", issue.Severity, issue.Path, issue.Message)
			}
		}
		for _, guidance := range report.Guidance {
			fmt.Fprintln(out, guidance)
		}
	}
	if report.SagaCheck != nil && !report.SagaCheck.Valid {
		return &StatusError{Code: 1}
	}
	return nil
}

func doctorModifiedText(modified *bool) string {
	if modified == nil {
		return "null"
	}
	return strconv.FormatBool(*modified)
}

func runningDoctorBuild() doctorBuild {
	build := doctorBuild{Version: VersionString(), Commit: Commit, BuildDate: BuildDate, GoVersion: runtime.Version()}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				build.VCSRevision = setting.Value
			case "vcs.modified":
				if value, err := strconv.ParseBool(setting.Value); err == nil {
					build.VCSModified = &value
				}
			}
		}
	}
	path, err := os.Executable()
	build.Executable = path
	if err == nil {
		path, err = filepath.EvalSymlinks(path)
		build.ResolvedExecutable = path
	}
	if err == nil {
		var file *os.File
		file, err = os.Open(path)
		if err == nil {
			defer file.Close()
			hash := sha256.New()
			if _, err = io.Copy(hash, file); err == nil {
				build.SHA256 = fmt.Sprintf("sha256:%x", hash.Sum(nil))
			}
		}
	}
	if err != nil {
		build.ExecutableError = err.Error()
	}
	return build
}
