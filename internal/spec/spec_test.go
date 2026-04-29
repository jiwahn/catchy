package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBundleHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{
		"ociVersion": "1.2.0",
		"hooks": {
			"prestart": [
				{
					"path": "/bin/echo",
					"args": ["echo", "hello"],
					"env": ["A=B"],
					"timeout": 3
				}
			]
		}
	}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	b, err := LoadBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.Hooks == nil || len(b.Hooks.Prestart) != 1 {
		t.Fatalf("expected one prestart hook, got %#v", b.Hooks)
	}
	h := b.Hooks.Prestart[0]
	if h.Path != "/bin/echo" || h.Args[1] != "hello" || h.Env[0] != "A=B" || h.Timeout != 3 {
		t.Fatalf("unexpected hook: %#v", h)
	}
}
