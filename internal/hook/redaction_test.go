package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactEnv(t *testing.T) {
	cfg := defaultRedactionConfig(nil)
	got := redactEnv([]string{
		"TOKEN=abc",
		"PASSWORD=abc",
		"MY_SECRET=abc",
		"NORMAL=abc",
	}, cfg)
	want := []string{
		"TOKEN=<redacted>",
		"PASSWORD=<redacted>",
		"MY_SECRET=<redacted>",
		"NORMAL=abc",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected env redaction:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestRedactNestedJSON(t *testing.T) {
	cfg := defaultRedactionConfig(nil)
	raw := json.RawMessage(`{
		"metadata": {
			"token": "abc",
			"nested": [
				{"password": "pw"},
				{"safe": "value"}
			]
		},
		"registry_auth": {
			"username": "u",
			"password": "p"
		},
		"safe": "ok"
	}`)

	var got map[string]any
	if err := json.Unmarshal(redactJSON(raw, cfg), &got); err != nil {
		t.Fatal(err)
	}
	metadata := got["metadata"].(map[string]any)
	if metadata["token"] != redactedValue {
		t.Fatalf("token was not redacted: %#v", metadata["token"])
	}
	nested := metadata["nested"].([]any)
	if nested[0].(map[string]any)["password"] != redactedValue {
		t.Fatalf("nested password was not redacted: %#v", nested[0])
	}
	if got["registry_auth"] != redactedValue {
		t.Fatalf("registry_auth object should be replaced: %#v", got["registry_auth"])
	}
	if got["safe"] != "ok" {
		t.Fatalf("safe value should remain: %#v", got["safe"])
	}
}

func TestRunWrapperRedactsByDefault(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho token=stdout-secret\necho password=stderr-secret >&2\n"), 0755); err != nil {
		t.Fatal(err)
	}

	traceDir := filepath.Join(dir, "traces")
	code := RunWrapper([]string{
		"--hook-stage", "prestart",
		"--orig-path", script,
		"--orig-args-json", `["hook.sh","token=arg-secret"]`,
		"--orig-env-json", `["TOKEN=env-secret","NORMAL=ok"]`,
		"--trace-dir", traceDir,
	}, bytes.NewBufferString(`{"annotations":{"api_key":"state-secret"},"safe":"ok"}`), &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	entry := readTraceEntry(t, traceDir)
	if !entry.Redacted || entry.TraceVersion != 2 {
		t.Fatalf("expected redacted trace version 2, got redacted=%v version=%d", entry.Redacted, entry.TraceVersion)
	}
	if strings.Contains(mustJSON(entry), "arg-secret") || strings.Contains(mustJSON(entry), "env-secret") || strings.Contains(mustJSON(entry), "state-secret") ||
		strings.Contains(mustJSON(entry), "stdout-secret") || strings.Contains(mustJSON(entry), "stderr-secret") {
		t.Fatalf("trace contains unredacted secret: %s", mustJSON(entry))
	}
	if !contains(entry.Env, "TOKEN=<redacted>") || !contains(entry.Env, "NORMAL=ok") {
		t.Fatalf("unexpected redacted env: %#v", entry.Env)
	}
}

func TestRunWrapperNoRedactDisablesRedaction(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho token=stdout-secret\n"), 0755); err != nil {
		t.Fatal(err)
	}

	traceDir := filepath.Join(dir, "traces")
	code := RunWrapper([]string{
		"--hook-stage", "prestart",
		"--orig-path", script,
		"--orig-env-json", `["TOKEN=env-secret"]`,
		"--trace-dir", traceDir,
		"--no-redact",
	}, bytes.NewBufferString(`{"token":"state-secret"}`), &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	entry := readTraceEntry(t, traceDir)
	if entry.Redacted {
		t.Fatalf("expected redaction disabled")
	}
	trace := mustJSON(entry)
	if !strings.Contains(trace, "env-secret") || !strings.Contains(trace, "state-secret") || !strings.Contains(trace, "stdout-secret") {
		t.Fatalf("expected unredacted values, got %s", trace)
	}
}

func TestRunWrapperCustomRedactKey(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho session_id=stdout-secret\n"), 0755); err != nil {
		t.Fatal(err)
	}

	traceDir := filepath.Join(dir, "traces")
	code := RunWrapper([]string{
		"--hook-stage", "prestart",
		"--orig-path", script,
		"--orig-env-json", `["SESSION_ID=env-secret"]`,
		"--trace-dir", traceDir,
		"--redact-key", "session_id",
	}, bytes.NewBufferString(`{"session_id":"state-secret"}`), &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	entry := readTraceEntry(t, traceDir)
	trace := mustJSON(entry)
	if strings.Contains(trace, "env-secret") || strings.Contains(trace, "state-secret") || strings.Contains(trace, "stdout-secret") {
		t.Fatalf("custom key did not redact trace: %s", trace)
	}
	if !contains(entry.RedactionKeys, "session_id") {
		t.Fatalf("custom redaction key not recorded: %#v", entry.RedactionKeys)
	}
}

func readTraceEntry(t *testing.T, traceDir string) TraceEntry {
	t.Helper()
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
	return entry
}
