package containerd

import (
	"errors"
	"os"
	"path/filepath"
)

// BundleLookupOptions controls filesystem-based runtime v2 bundle lookup.
type BundleLookupOptions struct {
	Root      string
	Namespace string
	ID        string
}

// BundleInfo describes a resolved containerd runtime v2 bundle path.
type BundleInfo struct {
	Root       string `json:"root"`
	Namespace  string `json:"namespace"`
	ID         string `json:"id"`
	BundlePath string `json:"bundlePath"`
	ConfigPath string `json:"configPath"`
	Exists     bool   `json:"exists"`
}

// DefaultRuntimeV2Root returns containerd's common runtime v2 task bundle root.
func DefaultRuntimeV2Root() string {
	return "/run/containerd/io.containerd.runtime.v2.task"
}

// FindBundle resolves a containerd runtime v2 bundle path by namespace and ID.
func FindBundle(opts BundleLookupOptions) (*BundleInfo, error) {
	if opts.Namespace == "" {
		return nil, errors.New("namespace is required")
	}
	if opts.ID == "" {
		return nil, errors.New("container id is required")
	}
	root := opts.Root
	if root == "" {
		root = DefaultRuntimeV2Root()
	}
	bundlePath := filepath.Join(root, opts.Namespace, opts.ID)
	configPath := filepath.Join(bundlePath, "config.json")
	info := &BundleInfo{
		Root:       root,
		Namespace:  opts.Namespace,
		ID:         opts.ID,
		BundlePath: bundlePath,
		ConfigPath: configPath,
		Exists:     configExists(configPath),
	}
	return info, nil
}

func configExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
