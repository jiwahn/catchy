package hook

// Package hook provides functionality to rewrite OCI hooks
// in a bundle's config.json to wrap them with a trace wrapper.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"catchy/internal/spec"
)

const (
	defaultWrapperPath = "/usr/local/bin/catchy-wrapper"
	backupFileName     = "config.json.catchy.bak"
)

// ErrNoHooks indicates that the bundle has no hooks to rewrite.
var ErrNoHooks = errors.New("no hooks found")

// WrapOptions configures how bundle hook rewriting is performed.
type WrapOptions struct {
	Force    bool
	TraceDir string
}

// WrapBundle rewrites every OCI hook in bundlePath/config.json to call wrapperPath.
func WrapBundle(bundlePath string, wrapperPath string) error {
	return WrapBundleWithOptions(bundlePath, wrapperPath, WrapOptions{})
}

// WrapBundleWithOptions rewrites every OCI hook in bundlePath/config.json to
// call wrapperPath, preserving the original config in config.json.catchy.bak.
func WrapBundleWithOptions(bundlePath string, wrapperPath string, opts WrapOptions) error {
	if wrapperPath == "" {
		wrapperPath = defaultWrapperPath
	}
	if opts.TraceDir == "" {
		opts.TraceDir = filepath.Join(bundlePath, ".catchy", "traces")
	}
	traceDir, err := filepath.Abs(opts.TraceDir)
	if err != nil {
		return fmt.Errorf("resolve trace dir: %w", err)
	}

	cfgPath := filepath.Join(bundlePath, "config.json")
	backupPath := filepath.Join(bundlePath, backupFileName)

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("config.json does not exist in bundle %q", bundlePath)
		}
		return fmt.Errorf("read config.json: %w", err)
	}

	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("unmarshal config.json: %w", err)
	}

	hooksRaw, ok := cfg["hooks"]
	if !ok || len(hooksRaw) == 0 || string(hooksRaw) == "null" {
		return ErrNoHooks
	}

	var hooks spec.Hooks
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		return fmt.Errorf("unmarshal hooks: %w", err)
	}

	changed := wrapHooks(&hooks, wrapperPath, traceDir)
	if !changed {
		return ErrNoHooks
	}

	if _, err := os.Stat(backupPath); err == nil && !opts.Force {
		return fmt.Errorf("backup already exists at %s; use --force to overwrite it", backupPath)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check backup: %w", err)
	}

	if err := copyFile(cfgPath, backupPath); err != nil {
		return fmt.Errorf("backup config.json: %w", err)
	}

	hooksOut, err := json.Marshal(hooks)
	if err != nil {
		return fmt.Errorf("marshal hooks: %w", err)
	}
	cfg["hooks"] = hooksOut

	out, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return fmt.Errorf("marshal config.json: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(cfgPath, out, 0644); err != nil {
		return fmt.Errorf("write config.json: %w", err)
	}

	return nil
}

// RestoreBundle restores bundlePath/config.json from config.json.catchy.bak.
func RestoreBundle(bundlePath string) error {
	cfgPath := filepath.Join(bundlePath, "config.json")
	backupPath := filepath.Join(bundlePath, backupFileName)

	if _, err := os.Stat(backupPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("backup does not exist at %s", backupPath)
		}
		return fmt.Errorf("check backup: %w", err)
	}

	if err := copyFile(backupPath, cfgPath); err != nil {
		return fmt.Errorf("restore config.json: %w", err)
	}

	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("remove backup: %w", err)
	}

	return nil
}

// RewriteHooks rewrites hooks in memory and returns the loaded bundle. Use
// WrapBundle when the rewritten config should be persisted.
func RewriteHooks(bundlePath string, wrapperPath string) (*spec.Bundle, error) {
	cfgPath := filepath.Join(bundlePath, "config.json")
	b, err := spec.LoadBundle(cfgPath)
	if err != nil {
		return nil, err
	}
	if b.Hooks != nil {
		wrapHooks(b.Hooks, wrapperPath, filepath.Join(bundlePath, ".catchy", "traces"))
	}
	return b, nil
}

// GenerateWrapper generates a trace wrapper binary and returns its
// path.  In the initial implementation this might simply be a
// symbolic link to a precompiled wrapper.  Here we return a
// placeholder path.
func GenerateWrapper(outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create wrapper dir: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate current executable: %w", err)
	}
	dst := filepath.Join(outputDir, fmt.Sprintf("catchy-wrapper-%d", time.Now().Unix()))
	if err := copyFile(self, dst); err != nil {
		return "", fmt.Errorf("copy wrapper: %w", err)
	}
	if err := os.Chmod(dst, 0755); err != nil {
		return "", fmt.Errorf("chmod wrapper: %w", err)
	}
	return dst, nil
}

func wrapHooks(hooks *spec.Hooks, wrapperPath string, traceDir string) bool {
	changed := false
	changed = wrapHookSlice("prestart", hooks.Prestart, wrapperPath, traceDir) || changed
	changed = wrapHookSlice("createRuntime", hooks.CreateRuntime, wrapperPath, traceDir) || changed
	changed = wrapHookSlice("createContainer", hooks.CreateContainer, wrapperPath, traceDir) || changed
	changed = wrapHookSlice("startContainer", hooks.StartContainer, wrapperPath, traceDir) || changed
	changed = wrapHookSlice("poststart", hooks.Poststart, wrapperPath, traceDir) || changed
	changed = wrapHookSlice("poststop", hooks.Poststop, wrapperPath, traceDir) || changed
	return changed
}

func wrapHookSlice(stage string, hooks []spec.Hook, wrapperPath string, traceDir string) bool {
	if len(hooks) == 0 {
		return false
	}

	for i := range hooks {
		orig := hooks[i]
		hooks[i] = wrapHook(stage, i, orig, wrapperPath, traceDir)
	}

	return true
}

func wrapHook(stage string, index int, orig spec.Hook, wrapperPath string, traceDir string) spec.Hook {
	indexString := strconv.Itoa(index)
	args := []string{
		filepath.Base(wrapperPath),
		"hook-wrapper",
		"--hook-stage", stage,
		"--hook-index", indexString,
		"--orig-path", orig.Path,
		"--trace-dir", traceDir,
	}

	if len(orig.Args) > 0 {
		args = append(args, "--orig-args-json", mustJSON(orig.Args))
	}
	if len(orig.Env) > 0 {
		args = append(args, "--orig-env-json", mustJSON(orig.Env))
	}
	if orig.Timeout > 0 {
		args = append(args, "--orig-timeout", strconv.Itoa(orig.Timeout))
	}

	env := append([]string{}, orig.Env...)
	env = append(env,
		"CATCHY_ORIG_PATH="+orig.Path,
		"CATCHY_HOOK_STAGE="+stage,
		"CATCHY_HOOK_INDEX="+indexString,
		"CATCHY_TRACE_DIR="+traceDir,
	)
	if len(orig.Args) > 0 {
		env = append(env, "CATCHY_ORIG_ARGS_JSON="+mustJSON(orig.Args))
	}
	if len(orig.Env) > 0 {
		env = append(env, "CATCHY_ORIG_ENV_JSON="+mustJSON(orig.Env))
	}
	if orig.Timeout > 0 {
		env = append(env, "CATCHY_ORIG_TIMEOUT="+strconv.Itoa(orig.Timeout))
	}

	return spec.Hook{
		Path:    wrapperPath,
		Args:    args,
		Env:     env,
		Timeout: orig.Timeout,
	}
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}

	return out.Close()
}
