package containerd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindBundleDefaultRootConstruction(t *testing.T) {
	info, err := FindBundle(BundleLookupOptions{Namespace: "default", ID: "test"})
	if err != nil {
		t.Fatalf("FindBundle: %v", err)
	}
	wantBundle := filepath.Join(DefaultRuntimeV2Root(), "default", "test")
	if info.Root != DefaultRuntimeV2Root() {
		t.Fatalf("Root = %q, want %q", info.Root, DefaultRuntimeV2Root())
	}
	if info.BundlePath != wantBundle {
		t.Fatalf("BundlePath = %q, want %q", info.BundlePath, wantBundle)
	}
	if info.ConfigPath != filepath.Join(wantBundle, "config.json") {
		t.Fatalf("ConfigPath = %q", info.ConfigPath)
	}
}

func TestFindBundleCustomRootConstruction(t *testing.T) {
	root := t.TempDir()
	info, err := FindBundle(BundleLookupOptions{Root: root, Namespace: "k8s.io", ID: "nginx"})
	if err != nil {
		t.Fatalf("FindBundle: %v", err)
	}
	want := filepath.Join(root, "k8s.io", "nginx")
	if info.BundlePath != want {
		t.Fatalf("BundlePath = %q, want %q", info.BundlePath, want)
	}
}

func TestFindBundleRequiresNamespace(t *testing.T) {
	if _, err := FindBundle(BundleLookupOptions{ID: "test"}); err == nil {
		t.Fatal("FindBundle returned nil error, want namespace error")
	}
}

func TestFindBundleRequiresID(t *testing.T) {
	if _, err := FindBundle(BundleLookupOptions{Namespace: "default"}); err == nil {
		t.Fatal("FindBundle returned nil error, want id error")
	}
}

func TestFindBundleExistsFalseWhenConfigMissing(t *testing.T) {
	root := t.TempDir()
	info, err := FindBundle(BundleLookupOptions{Root: root, Namespace: "default", ID: "missing"})
	if err != nil {
		t.Fatalf("FindBundle: %v", err)
	}
	if info.Exists {
		t.Fatal("Exists = true, want false")
	}
}

func TestFindBundleExistsTrueWhenConfigExists(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "default", "test", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	info, err := FindBundle(BundleLookupOptions{Root: root, Namespace: "default", ID: "test"})
	if err != nil {
		t.Fatalf("FindBundle: %v", err)
	}
	if !info.Exists {
		t.Fatal("Exists = false, want true")
	}
}
