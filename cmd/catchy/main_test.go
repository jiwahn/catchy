package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"catchy/internal/hook"
	"catchy/internal/spec"
)

func TestPrintHooksRedactsByDefault(t *testing.T) {
	var out bytes.Buffer
	printHooks(&out, "prestart", []spec.Hook{
		{
			Path: "/bin/hook",
			Args: []string{"hook", "token=arg-secret", "safe"},
			Env:  []string{"TOKEN=env-secret", "NORMAL=ok", "MY_SECRET=hidden"},
		},
	}, hook.NewRedactionConfig(true, nil))

	got := out.String()
	if strings.Contains(got, "arg-secret") || strings.Contains(got, "env-secret") || strings.Contains(got, "hidden") {
		t.Fatalf("inspect output leaked secret:\n%s", got)
	}
	for _, want := range []string{"token=<redacted>", "TOKEN=<redacted>", "NORMAL=ok", "MY_SECRET=<redacted>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, got)
		}
	}
}

func TestPrintHooksNoRedact(t *testing.T) {
	var out bytes.Buffer
	printHooks(&out, "prestart", []spec.Hook{
		{
			Path: "/bin/hook",
			Args: []string{"hook", "token=arg-secret"},
			Env:  []string{"TOKEN=env-secret"},
		},
	}, hook.NewRedactionConfig(false, nil))

	got := out.String()
	if !strings.Contains(got, "arg-secret") || !strings.Contains(got, "env-secret") {
		t.Fatalf("inspect output should show original values when redaction is disabled:\n%s", got)
	}
}

func TestPrintHooksCustomRedactKey(t *testing.T) {
	var out bytes.Buffer
	printHooks(&out, "prestart", []spec.Hook{
		{
			Path: "/bin/hook",
			Args: []string{"hook", "session_id=arg-secret"},
			Env:  []string{"SESSION_ID=env-secret"},
		},
	}, hook.NewRedactionConfig(true, []string{"session_id"}))

	got := out.String()
	if strings.Contains(got, "arg-secret") || strings.Contains(got, "env-secret") {
		t.Fatalf("inspect output leaked custom-key secret:\n%s", got)
	}
	if !strings.Contains(got, "session_id=<redacted>") || !strings.Contains(got, "SESSION_ID=<redacted>") {
		t.Fatalf("inspect output missing custom-key redactions:\n%s", got)
	}
}

func TestBuildRuntimeCommandArgs(t *testing.T) {
	got := buildRuntimeCommandArgs("--debug --root /legacy/root", []string{"--root", "/tmp/runc root"}, "bundle", "id1")
	want := []string{"--debug", "--root", "/legacy/root", "--root", "/tmp/runc root", "run", "-b", "bundle", "id1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected runtime args:\ngot  %#v\nwant %#v", got, want)
	}
}
