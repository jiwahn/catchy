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
		"hints:",
		"  * required environment variable appears to be missing. Check hook env configuration in config.json or the invoking runtime.",
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
			Signal:     "SIGILL",
			DurationMs: 5,
		},
	}})

	if got := result.Failures[0].LikelyCause; got != "hook terminated by signal" {
		t.Fatalf("unexpected cause: %q", got)
	}
	assertHint(t, result.Failures[0].Hints, "hook process hit an illegal CPU instruction")
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
	assertHint(t, result.Failures[0].Hints, "hook exceeded its configured timeout")
}

func TestDiagnoseJSONOutput(t *testing.T) {
	result := FromReport(&report.Report{Entries: []report.Entry{
		{
			HookStage:  "prestart",
			HookIndex:  0,
			Path:       "/bin/false",
			ExitCode:   127,
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
	if decoded.Failures[0].Hints == nil {
		t.Fatalf("expected hints field to be present in JSON failure")
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

func TestDiagnoseHints(t *testing.T) {
	tests := []struct {
		name  string
		entry report.Entry
		want  string
	}{
		{
			name: "permission denied",
			entry: report.Entry{
				HookStage: "prestart",
				Path:      "/hooks/setup",
				Error:     "fork/exec /hooks/setup: permission denied",
			},
			want: "hook path or one of its referenced files may not be executable or accessible",
		},
		{
			name: "no such file",
			entry: report.Entry{
				HookStage: "prestart",
				Path:      "/missing/hook",
				ExitCode:  127,
				Error:     `exec: "/missing/hook": stat /missing/hook: no such file or directory`,
			},
			want: "hook executable or interpreter may be missing on the host",
		},
		{
			name: "executable file not found",
			entry: report.Entry{
				HookStage: "prestart",
				Error:     `exec: "hook": executable file not found in $PATH`,
			},
			want: "hook executable could not be resolved",
		},
		{
			name: "exec format error",
			entry: report.Entry{
				HookStage: "prestart",
				Error:     "fork/exec /hooks/setup: exec format error",
			},
			want: "wrong architecture or may be missing a valid shebang",
		},
		{
			name: "sigkill signal",
			entry: report.Entry{
				HookStage: "prestart",
				Signal:    "SIGKILL",
			},
			want: "hook was killed by SIGKILL",
		},
		{
			name: "sigkill text",
			entry: report.Entry{
				HookStage: "prestart",
				Error:     "hook was killed by signal 9",
			},
			want: "hook was killed by SIGKILL",
		},
		{
			name: "deadline text",
			entry: report.Entry{
				HookStage: "prestart",
				Error:     "context deadline exceeded",
			},
			want: "hook exceeded its configured timeout",
		},
		{
			name: "missing env is not set",
			entry: report.Entry{
				HookStage: "prestart",
				ExitCode:  1,
				Stderr:    "GPU_DEVICE_ID is not set",
			},
			want: "required environment variable appears to be missing",
		},
		{
			name: "missing required env",
			entry: report.Entry{
				HookStage: "prestart",
				ExitCode:  1,
				Stderr:    "missing required NVIDIA_VISIBLE_DEVICES",
			},
			want: "required environment variable appears to be missing",
		},
		{
			name: "exit 126",
			entry: report.Entry{
				HookStage: "prestart",
				ExitCode:  126,
			},
			want: "command was found but could not be executed",
		},
		{
			name: "exit 127",
			entry: report.Entry{
				HookStage: "prestart",
				ExitCode:  127,
			},
			want: "command was not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromReport(&report.Report{Entries: []report.Entry{tt.entry}})
			if result.FailedTraces != 1 {
				t.Fatalf("expected one failure, got %#v", result)
			}
			assertHint(t, result.Failures[0].Hints, tt.want)
			if text := result.FormatText(); !strings.Contains(text, tt.want) {
				t.Fatalf("text output missing hint %q:\n%s", tt.want, text)
			}
		})
	}
}

func TestDiagnoseHintFalsePositives(t *testing.T) {
	tests := []struct {
		name    string
		entry   report.Entry
		notWant string
	}{
		{
			name: "generic killed text",
			entry: report.Entry{
				HookStage: "prestart",
				ExitCode:  1,
				Stderr:    "process was not killed",
			},
			notWant: "hook was killed by SIGKILL",
		},
		{
			name: "set namespace",
			entry: report.Entry{
				HookStage: "prestart",
				ExitCode:  1,
				Stderr:    "set namespace failed",
			},
			notWant: "required environment variable appears to be missing",
		},
		{
			name: "set mount propagation",
			entry: report.Entry{
				HookStage: "prestart",
				ExitCode:  1,
				Stderr:    "set mount propagation failed",
			},
			notWant: "required environment variable appears to be missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromReport(&report.Report{Entries: []report.Entry{tt.entry}})
			if result.FailedTraces != 1 {
				t.Fatalf("expected one failure, got %#v", result)
			}
			for _, hint := range result.Failures[0].Hints {
				if strings.Contains(hint, tt.notWant) {
					t.Fatalf("unexpected hint %q in %#v", tt.notWant, result.Failures[0].Hints)
				}
			}
		})
	}
}

func TestDiagnoseMultipleFailures(t *testing.T) {
	result := FromReport(&report.Report{Entries: []report.Entry{
		{HookStage: "prestart", HookIndex: 0, Path: "/missing", ExitCode: 127},
		{HookStage: "poststop", HookIndex: 1, Path: "/slow", TimedOut: true},
	}})

	if result.TotalTraces != 2 || result.FailedTraces != 2 || len(result.Failures) != 2 {
		t.Fatalf("unexpected multiple failure result: %#v", result)
	}
	text := result.FormatText()
	if !strings.Contains(text, "prestart[0] failed") || !strings.Contains(text, "poststop[1] failed") {
		t.Fatalf("text output missing failures:\n%s", text)
	}
}

func assertHint(t *testing.T, hints []string, wantSubstring string) {
	t.Helper()
	for _, hint := range hints {
		if strings.Contains(hint, wantSubstring) {
			return
		}
	}
	t.Fatalf("missing hint containing %q in %#v", wantSubstring, hints)
}
