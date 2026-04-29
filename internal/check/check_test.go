package check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCheckBundleNoHooks(t *testing.T) {
	bundle := writeBundle(t, `{}`)

	result, err := CheckBundle(bundle)
	if err != nil {
		t.Fatalf("CheckBundle: %v", err)
	}
	if result.HasHooks {
		t.Fatalf("HasHooks = true, want false")
	}
	if result.HasProblems() {
		t.Fatalf("HasProblems = true, want false")
	}
	if got := result.FormatText(); !strings.Contains(got, "hooks: none") {
		t.Fatalf("FormatText() = %q, want hooks: none", got)
	}
}

func TestCheckBundleHookProblems(t *testing.T) {
	dir := t.TempDir()
	notExecutable := writeFile(t, filepath.Join(dir, "not-executable.sh"), "#!/bin/sh\n", 0644)
	missingInterpreter := writeFile(t, filepath.Join(dir, "missing-interpreter.sh"), "#!/definitely/missing/catchy-test-interpreter\n", 0755)

	tests := []struct {
		name     string
		hookPath string
		timeout  int
		wantCode string
	}{
		{
			name:     "empty hook path",
			hookPath: "",
			wantCode: "empty_path",
		},
		{
			name:     "non-absolute path",
			hookPath: "relative-hook",
			wantCode: "non_absolute_path",
		},
		{
			name:     "missing path",
			hookPath: filepath.Join(dir, "missing-hook"),
			wantCode: "path_missing",
		},
		{
			name:     "path is directory",
			hookPath: dir,
			wantCode: "path_is_directory",
		},
		{
			name:     "not executable",
			hookPath: notExecutable,
			wantCode: "not_executable",
		},
		{
			name:     "script with missing direct interpreter",
			hookPath: missingInterpreter,
			wantCode: "interpreter_missing",
		},
		{
			name:     "negative timeout",
			hookPath: executableHook(t, dir, "negative-timeout.sh", "#!/bin/sh\nexit 0\n"),
			timeout:  -1,
			wantCode: "negative_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := writeBundle(t, hookConfig(tt.hookPath, tt.timeout))

			result, err := CheckBundle(bundle)
			if err != nil {
				t.Fatalf("CheckBundle: %v", err)
			}
			if !result.HasProblems() {
				t.Fatalf("HasProblems = false, want true")
			}
			check := singleCheck(t, result)
			if !hasProblem(check.Problems, tt.wantCode) {
				t.Fatalf("problem codes = %v, want %q", problemCodes(check.Problems), tt.wantCode)
			}
		})
	}
}

func TestCheckBundleExecutableHookOK(t *testing.T) {
	dir := t.TempDir()
	hook := executableHook(t, dir, "ok.sh", "#!/bin/sh\nexit 0\n")
	bundle := writeBundle(t, hookConfig(hook, 0))

	result, err := CheckBundle(bundle)
	if err != nil {
		t.Fatalf("CheckBundle: %v", err)
	}
	check := singleCheck(t, result)
	if result.HasProblems() {
		t.Fatalf("HasProblems = true, problems = %v", check.Problems)
	}
	if len(check.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", check.Warnings)
	}
	if got := result.FormatText(); !strings.Contains(got, "status: ok") {
		t.Fatalf("FormatText() = %q, want status: ok", got)
	}
}

func TestCheckBundleScriptWithUsrBinEnv(t *testing.T) {
	if _, err := os.Stat("/usr/bin/env"); err != nil {
		t.Skip("/usr/bin/env is not available on this host")
	}
	dir := t.TempDir()
	hook := executableHook(t, dir, "env.sh", "#!/usr/bin/env sh\nexit 0\n")
	bundle := writeBundle(t, hookConfig(hook, 0))

	result, err := CheckBundle(bundle)
	if err != nil {
		t.Fatalf("CheckBundle: %v", err)
	}
	check := singleCheck(t, result)
	if hasProblem(check.Problems, "interpreter_missing") {
		t.Fatalf("unexpected interpreter_missing problem: %v", check.Problems)
	}
}

func TestCheckBundleLargeTimeoutWarning(t *testing.T) {
	dir := t.TempDir()
	hook := executableHook(t, dir, "slow.sh", "#!/bin/sh\nexit 0\n")
	bundle := writeBundle(t, hookConfig(hook, largeTimeoutSeconds+1))

	result, err := CheckBundle(bundle)
	if err != nil {
		t.Fatalf("CheckBundle: %v", err)
	}
	if result.HasProblems() {
		t.Fatalf("HasProblems = true, want false")
	}
	check := singleCheck(t, result)
	if !hasProblem(check.Warnings, "large_timeout") {
		t.Fatalf("warning codes = %v, want large_timeout", problemCodes(check.Warnings))
	}
}

func TestCheckBundleJSONOutputCanBeMarshaled(t *testing.T) {
	dir := t.TempDir()
	hook := executableHook(t, dir, "ok.sh", "#!/bin/sh\nexit 0\n")
	bundle := writeBundle(t, hookConfig(hook, 0))

	result, err := CheckBundle(bundle)
	if err != nil {
		t.Fatalf("CheckBundle: %v", err)
	}
	var decoded Result
	if err := json.Unmarshal([]byte(result.FormatJSON()), &decoded); err != nil {
		t.Fatalf("json.Unmarshal FormatJSON: %v", err)
	}
	if len(decoded.Checks) != 1 {
		t.Fatalf("decoded checks = %d, want 1", len(decoded.Checks))
	}
}

func writeBundle(t *testing.T, hooksJSON string) string {
	t.Helper()
	dir := t.TempDir()
	config := `{"ociVersion":"1.2.0","hooks":` + hooksJSON + `}`
	if hooksJSON == `{}` {
		config = `{"ociVersion":"1.2.0"}`
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	return dir
}

func hookConfig(path string, timeout int) string {
	return `{"prestart":[{"path":` + strconv.Quote(path) + `,"timeout":` + strconv.Itoa(timeout) + `}]}`
}

func executableHook(t *testing.T, dir, name, content string) string {
	t.Helper()
	return writeFile(t, filepath.Join(dir, name), content, 0755)
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
	return path
}

func singleCheck(t *testing.T, result *Result) HookCheck {
	t.Helper()
	if len(result.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(result.Checks))
	}
	return result.Checks[0]
}

func hasProblem(problems []Problem, code string) bool {
	for _, problem := range problems {
		if problem.Code == code {
			return true
		}
	}
	return false
}

func problemCodes(problems []Problem) []string {
	var codes []string
	for _, problem := range problems {
		codes = append(codes, problem.Code)
	}
	return codes
}
