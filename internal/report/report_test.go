package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDirAndFormatText(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
		"timestamp": "2026-04-29T01:02:03Z",
		"hookStage": "prestart",
		"hookIndex": 0,
		"path": "/bin/false",
		"durationMs": 12,
		"exitCode": 1,
		"stderr": "boom\n",
		"traceVersion": 1
	}`)
	if err := os.WriteFile(filepath.Join(dir, "trace.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(r.Entries))
	}
	text := r.FormatText()
	if !strings.Contains(text, "prestart[0] failed") || !strings.Contains(text, "stderr: boom") {
		t.Fatalf("unexpected text report:\n%s", text)
	}
	if !strings.Contains(r.FormatJSON(), `"hookStage": "prestart"`) {
		t.Fatalf("unexpected json report")
	}
	if !strings.Contains(r.FormatYAML(), `hookStage: "prestart"`) {
		t.Fatalf("unexpected yaml report")
	}
}
