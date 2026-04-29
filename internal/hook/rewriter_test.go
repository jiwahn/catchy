package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"catchy/internal/spec"
)

func TestWrapBundleWithOptionsAndRestore(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	original := []byte(`{
		"ociVersion": "1.2.0",
		"hooks": {
			"prestart": [
				{
					"path": "/bin/echo",
					"args": ["echo", "hello"],
					"env": ["A=B"],
					"timeout": 4
				}
			]
		}
	}`)
	if err := os.WriteFile(configPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	traceDir := filepath.Join(dir, "traces")
	if err := WrapBundleWithOptions(dir, "/tmp/catchy", WrapOptions{TraceDir: traceDir}); err != nil {
		t.Fatal(err)
	}

	b, err := spec.LoadBundle(configPath)
	if err != nil {
		t.Fatal(err)
	}
	h := b.Hooks.Prestart[0]
	if h.Path != "/tmp/catchy" {
		t.Fatalf("expected wrapped path, got %q", h.Path)
	}
	if len(h.Args) < 2 || h.Args[1] != "hook-wrapper" {
		t.Fatalf("expected wrapper subcommand in args, got %#v", h.Args)
	}
	if !contains(h.Env, "CATCHY_TRACE_DIR="+traceDir) {
		t.Fatalf("expected trace dir env, got %#v", h.Env)
	}

	if err := RestoreBundle(dir); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var restoredJSON, originalJSON map[string]any
	if err := json.Unmarshal(restored, &restoredJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(original, &originalJSON); err != nil {
		t.Fatal(err)
	}
	if restoredJSON["hooks"] == nil || originalJSON["hooks"] == nil {
		t.Fatalf("expected hooks after restore")
	}
}

func TestWrapBundleNoHooks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"ociVersion":"1.2.0"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WrapBundleWithOptions(dir, "/tmp/catchy", WrapOptions{}); err != ErrNoHooks {
		t.Fatalf("expected ErrNoHooks, got %v", err)
	}
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
