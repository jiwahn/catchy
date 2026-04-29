package diagnose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiwahn/catchy/internal/report"
)

func TestDiagnoseNoTraceFiles(t *testing.T) {
	result, err := ParseDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalTraces != 0 || result.FailedTraces != 0 || len(result.Failures) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := result.FormatText(); got != "no hook traces found\n" {
		t.Fatalf("unexpected text: %q", got)
	}
}

func TestDiagnoseSuccessfulTraceOnly(t *testing.T) {
	result := FromReport(&report.Report{Entries: []report.Entry{
		{
			HookStage:  "prestart",
			HookIndex:  0,
			Path:       "/bin/true",
			ExitCode:   0,
			DurationMs: 2,
		},
	}})

	if result.TotalTraces != 1 || result.FailedTraces != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := result.FormatText(); got != "no hook failures found\ntraces: 1\n" {
		t.Fatalf("unexpected text: %q", got)
	}
}

func TestDiagnoseNonZeroExit(t *testing.T) {
	result := FromReport(&report.Report{Entries: []report.Entry{
		{
			HookStage:  "prestart",
			HookIndex:  0,
			Path:       "/bin/sh",
			ExitCode:   42,
			DurationMs: 3,
			Stderr:     "demo prestart hook: missing required GPU_DEVICE_ID\nhint: fix config\n",
			Stdout:     "partial stdout\n",
			Redacted:   true,
			File:       "/tmp/trace.json",
		},
	}})

	if result.FailedTraces != 1 {
		t.Fatalf("expected one failure, got %#v", result)
	}
	failure := result.Failures[0]
	if failure.LikelyCause != "hook exited with non-zero status" {
		t.Fatalf("unexpected cause: %q", failure.LikelyCause)
	}
	text := result.FormatText()
	for _, want := range []string{
		"hook failures: 1 of 1 traces",
		"prestart[0] failed",
		"exit: 42",
		"redacted: true",
		"likely cause: hook exited with non-zero status",
		"stderr: demo prestart hook: missing required GPU_DEVICE_ID\\nhint: fix config",
		"stdout: partial stdout",
		"trace: /tmp/trace.json",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("diagnosis text missing %q:\n%s", want, text)
		}
	}
}

func TestDiagnoseSignal(t *testing.T) {
	result := FromReport(&report.Report{Entries: []report.Entry{
		{
			HookStage:  "createRuntime",
			HookIndex:  1,
			Path:       "/bin/hook",
			ExitCode:   -1,
			Signal:     "killed",
			DurationMs: 5,
		},
	}})

	if got := result.Failures[0].LikelyCause; got != "hook terminated by signal" {
		t.Fatalf("unexpected cause: %q", got)
	}
}

func TestDiagnoseTimeout(t *testing.T) {
	result := FromReport(&report.Report{Entries: []report.Entry{
		{
			HookStage:  "startContainer",
			HookIndex:  0,
			Path:       "/bin/hook",
			TimedOut:   true,
			DurationMs: 5000,
		},
	}})

	if got := result.Failures[0].LikelyCause; got != "hook timed out" {
		t.Fatalf("unexpected cause: %q", got)
	}
	if text := result.FormatText(); !strings.Contains(text, "timed out: true") {
		t.Fatalf("diagnosis text missing timeout:\n%s", text)
	}
}

func TestDiagnoseJSONOutput(t *testing.T) {
	result := FromReport(&report.Report{Entries: []report.Entry{
		{
			HookStage:  "prestart",
			HookIndex:  0,
			Path:       "/bin/false",
			ExitCode:   1,
			DurationMs: 1,
		},
	}})

	var decoded Result
	if err := json.Unmarshal([]byte(result.FormatJSON()), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TotalTraces != 1 || decoded.FailedTraces != 1 || decoded.Failures[0].LikelyCause != "hook exited with non-zero status" {
		t.Fatalf("unexpected decoded result: %#v", decoded)
	}
}

func TestParseDirReadsTraceFiles(t *testing.T) {
	dir := t.TempDir()
	entry := report.Entry{
		Timestamp:    time.Now().UTC(),
		HookStage:    "prestart",
		HookIndex:    0,
		Path:         "/bin/false",
		ExitCode:     1,
		DurationMs:   1,
		TraceVersion: 2,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "trace.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalTraces != 1 || result.FailedTraces != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}
