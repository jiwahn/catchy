package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeRunCapturesPrestartHook(t *testing.T) {
	if os.Getenv("CATCHY_E2E_RUNTIME") == "" {
		t.Skip("set CATCHY_E2E_RUNTIME=1 to run OCI runtime integration tests")
	}

	repo := repoRoot(t)
	catchy := buildCatchy(t, repo)
	rootfs := filepath.Join(repo, "bundle", "rootfs")

	for _, runtimeName := range e2eRuntimes() {
		runtimePath, err := exec.LookPath(runtimeName)
		if err != nil {
			t.Run(runtimeName, func(t *testing.T) {
				t.Skipf("%s not found in PATH", runtimeName)
			})
			continue
		}

		t.Run(runtimeName, func(t *testing.T) {
			bundle := t.TempDir()
			traceDir := filepath.Join(bundle, "traces")
			hookEvidence := filepath.Join(bundle, "hook-ran.txt")
			hookPath := filepath.Join(bundle, "prestart-hook.sh")
			hookScript := "#!/bin/sh\n" +
				"echo hook stdout\n" +
				"echo hook stderr >&2\n" +
				"cat > " + shellQuote(filepath.Join(bundle, "state.json")) + "\n" +
				"echo ran > " + shellQuote(hookEvidence) + "\n"
			if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
				t.Fatal(err)
			}

			generateRootlessSpec(t, runtimePath, bundle)
			configPath := filepath.Join(bundle, "config.json")
			patchConfig(t, configPath, rootfs, hookPath)

			cmd := exec.Command(catchy, "run",
				"--runtime", runtimePath,
				"--wrapper", catchy,
				"--trace-dir", traceDir,
				"--id", "catchy-e2e-"+runtimeName+"-"+strings.ReplaceAll(t.Name(), "/", "-"),
				bundle,
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				if isRuntimePermissionFailure(string(out)) {
					t.Skipf("%s cannot run containers in this environment:\n%s", runtimeName, out)
				}
				t.Fatalf("catchy run failed with %s:\n%s", runtimeName, out)
			}

			if _, err := os.Stat(hookEvidence); err != nil {
				t.Fatalf("hook evidence was not written: %v", err)
			}
			assertBundleRestored(t, configPath, hookPath)
			assertTrace(t, traceDir, hookPath)
		})
	}
}

func e2eRuntimes() []string {
	raw := os.Getenv("CATCHY_E2E_RUNTIMES")
	if strings.TrimSpace(raw) == "" {
		return []string{"runc", "crun"}
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	var runtimes []string
	for _, f := range fields {
		if f != "" {
			runtimes = append(runtimes, f)
		}
	}
	return runtimes
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func buildCatchy(t *testing.T, repo string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "catchy")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/catchy")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed:\n%s", out)
	}
	return bin
}

func generateRootlessSpec(t *testing.T, runtimePath string, bundle string) {
	t.Helper()
	cmd := exec.Command(runtimePath, "spec", "--rootless", "--bundle", bundle)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s spec failed:\n%s", runtimePath, out)
	}
}

func patchConfig(t *testing.T, configPath string, rootfs string, hookPath string) {
	t.Helper()
	var cfg map[string]any
	readJSON(t, configPath, &cfg)

	cfg["root"] = map[string]any{
		"path":     rootfs,
		"readonly": true,
	}
	cfg["process"] = map[string]any{
		"terminal": false,
		"user": map[string]any{
			"uid": float64(os.Getuid()),
			"gid": float64(os.Getgid()),
		},
		"args": []any{"/bin/true"},
		"env":  []any{"PATH=/bin:/usr/bin:/sbin:/usr/sbin"},
		"cwd":  "/",
	}
	cfg["hooks"] = map[string]any{
		"prestart": []any{
			map[string]any{
				"path": hookPath,
				"args": []any{hookPath},
				"env":  []any{"CATCHY_E2E=1"},
			},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func assertBundleRestored(t *testing.T, configPath string, hookPath string) {
	t.Helper()
	var cfg struct {
		Hooks struct {
			Prestart []struct {
				Path string `json:"path"`
			} `json:"prestart"`
		} `json:"hooks"`
	}
	readJSON(t, configPath, &cfg)
	if len(cfg.Hooks.Prestart) != 1 || cfg.Hooks.Prestart[0].Path != hookPath {
		t.Fatalf("bundle was not restored to original hook path: %#v", cfg.Hooks.Prestart)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(configPath), "config.json.catchy.bak")); !os.IsNotExist(err) {
		t.Fatalf("backup should be removed after restore, stat err=%v", err)
	}
}

func assertTrace(t *testing.T, traceDir string, hookPath string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(traceDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one trace file, got %d in %s", len(files), traceDir)
	}

	var trace struct {
		HookStage string `json:"hookStage"`
		HookIndex int    `json:"hookIndex"`
		Path      string `json:"path"`
		ExitCode  int    `json:"exitCode"`
		Stdout    string `json:"stdout"`
		Stderr    string `json:"stderr"`
		State     any    `json:"state"`
	}
	readJSON(t, files[0], &trace)
	if trace.HookStage != "prestart" || trace.HookIndex != 0 || trace.Path != hookPath || trace.ExitCode != 0 {
		t.Fatalf("unexpected trace metadata: %#v", trace)
	}
	if !strings.Contains(trace.Stdout, "hook stdout") || !strings.Contains(trace.Stderr, "hook stderr") {
		t.Fatalf("trace did not capture hook output: %#v", trace)
	}
	if trace.State == nil {
		t.Fatalf("trace did not capture OCI hook state")
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func isRuntimePermissionFailure(output string) bool {
	lower := strings.ToLower(output)
	needles := []string{
		"operation not permitted",
		"permission denied",
		"failed to unshare",
		"setresuid",
		"rootless",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
