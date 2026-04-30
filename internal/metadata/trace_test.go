package metadata

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseManifestAnnotations(t *testing.T) {
	data := []byte(`{
		"schemaVersion": 2,
		"annotations": {
			"com.example.one": "yes",
			"com.example.two": "also"
		}
	}`)

	got := ParseManifestAnnotations(data)
	if got["com.example.one"] != "yes" {
		t.Fatalf("annotation = %q, want yes", got["com.example.one"])
	}
	if got["com.example.two"] != "also" {
		t.Fatalf("annotation = %q, want also", got["com.example.two"])
	}
}

func TestParseConfigLabels(t *testing.T) {
	data := []byte(`{
		"config": {
			"Labels": {
				"nginx": "nope",
				"role": "test"
			}
		}
	}`)

	got := ParseConfigLabels(data)
	if got["nginx"] != "nope" {
		t.Fatalf("label = %q, want nope", got["nginx"])
	}
	if got["role"] != "test" {
		t.Fatalf("label = %q, want test", got["role"])
	}
}

func TestParseDockerLabels(t *testing.T) {
	data := []byte(`[{
		"Config": {
			"Labels": {
				"docker.label": "value"
			}
		}
	}]`)

	got := parseDockerLabels(data)
	if got["docker.label"] != "value" {
		t.Fatalf("label = %q, want value", got["docker.label"])
	}
}

func TestParseSkopeoLabels(t *testing.T) {
	data := []byte(`{
		"Labels": {
			"skopeo.label": "value"
		}
	}`)

	got := parseSkopeoLabels(data)
	if got["skopeo.label"] != "value" {
		t.Fatalf("label = %q, want value", got["skopeo.label"])
	}
}

func TestBuildObservations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
		wantMessage string
		wantHint    string
	}{
		{
			name:        "manifest annotations only",
			annotations: map[string]string{"com.example": "yes"},
			wantMessage: "manifest annotations present but not reflected in config labels",
			wantHint:    "many runtimes (including containerd) do not propagate manifest annotations into runtime configuration",
		},
		{
			name:        "config labels only",
			labels:      map[string]string{"label": "yes"},
			wantMessage: "config labels present",
			wantHint:    "labels are typically available at container runtime level, unlike manifest annotations",
		},
		{
			name:        "both empty",
			wantMessage: "no annotations or labels found in image metadata",
			wantHint:    "image may not include custom metadata; hook-based features relying on annotations will not work",
		},
		{
			name:        "both present",
			annotations: map[string]string{"annotation": "yes"},
			labels:      map[string]string{"label": "yes"},
			wantMessage: "both manifest annotations and config labels present",
			wantHint:    "verify which fields your runtime actually propagates (containerd typically uses config labels, not manifest annotations)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildObservations(tt.annotations, tt.labels)
			if len(got) != 1 {
				t.Fatalf("observations = %d, want 1", len(got))
			}
			if got[0].Message != tt.wantMessage {
				t.Fatalf("message = %q, want %q", got[0].Message, tt.wantMessage)
			}
			if got[0].Hint != tt.wantHint {
				t.Fatalf("hint = %q, want %q", got[0].Hint, tt.wantHint)
			}
		})
	}
}

func TestTraceImageUsesCraneWhenAvailable(t *testing.T) {
	restore := replaceCommandHooks(
		func(name string) (string, error) {
			if name == "crane" {
				return "/usr/bin/crane", nil
			}
			return "", errors.New("not found")
		},
		func(name string, args ...string) ([]byte, []byte, error) {
			if name != "crane" {
				t.Fatalf("command = %s, want crane", name)
			}
			switch {
			case reflect.DeepEqual(args, []string{"manifest", "example.com/test:latest"}):
				return []byte(`{"annotations":{"com.example":"yes"}}`), nil, nil
			case reflect.DeepEqual(args, []string{"config", "example.com/test:latest"}):
				return []byte(`{"config":{"Labels":{"role":"test"}}}`), nil, nil
			default:
				t.Fatalf("unexpected args: %v", args)
			}
			return nil, nil, nil
		},
	)
	defer restore()

	trace, err := TraceImage("example.com/test:latest")
	if err != nil {
		t.Fatalf("TraceImage: %v", err)
	}
	if trace.ManifestAnnotations["com.example"] != "yes" {
		t.Fatalf("annotation = %q, want yes", trace.ManifestAnnotations["com.example"])
	}
	if trace.ConfigLabels["role"] != "test" {
		t.Fatalf("label = %q, want test", trace.ConfigLabels["role"])
	}
}

func TestTraceImageNoSupportedTool(t *testing.T) {
	restore := replaceCommandHooks(
		func(name string) (string, error) {
			return "", errors.New("not found")
		},
		nil,
	)
	defer restore()

	_, err := TraceImage("example.com/test:latest")
	if err == nil {
		t.Fatal("TraceImage returned nil error, want unsupported tool error")
	}
	if err.Error() != "no supported tool found (crane, skopeo, docker)\nhint: install crane (recommended) or skopeo to enable metadata tracing" {
		t.Fatalf("error = %q", err.Error())
	}
}

func replaceCommandHooks(find func(string) (string, error), run commandRunner) func() {
	oldLookPath := lookPath
	oldRunCmd := runCmd
	lookPath = find
	if run != nil {
		runCmd = run
	}
	return func() {
		lookPath = oldLookPath
		runCmd = oldRunCmd
	}
}
