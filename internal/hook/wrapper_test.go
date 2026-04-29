package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWrapperWritesTraceAndMirrorsOutput(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat >/dev/null\necho out:$1\necho err:$A >&2\n"), 0755); err != nil {
		t.Fatal(err)
	}

	traceDir := filepath.Join(dir, "traces")
	var stdout, stderr bytes.Buffer
	code := RunWrapper([]string{
		"--hook-stage", "prestart",
		"--hook-index", "2",
		"--orig-path", script,
		"--orig-args-json", `["hook.sh","arg"]`,
		"--orig-env-json", `["A=B"]`,
		"--trace-dir", traceDir,
	}, bytes.NewBufferString(`{"pid":123}`), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, stderr.String())
	}
	if stdout.String() != "out:arg\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "err:B\n" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	files, err := filepath.Glob(filepath.Join(traceDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one trace, got %d", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var entry TraceEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	if entry.HookStage != "prestart" || entry.HookIndex != 2 || entry.Path != script || entry.ExitCode != 0 {
		t.Fatalf("unexpected trace entry: %#v", entry)
	}
	if entry.Stdout != "out:arg\n" || entry.Stderr != "err:B\n" {
		t.Fatalf("trace did not capture output: %#v", entry)
	}
}

func TestRunWrapperPropagatesExitCode(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}

	code := RunWrapper([]string{
		"--hook-stage", "prestart",
		"--orig-path", script,
		"--trace-dir", filepath.Join(dir, "traces"),
	}, bytes.NewBufferString(`{}`), &bytes.Buffer{}, &bytes.Buffer{})
	if code != 7 {
		t.Fatalf("expected exit 7, got %d", code)
	}
}
