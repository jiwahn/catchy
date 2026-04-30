package metadata

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseManifestAnnotations(t *testing.T) {
	data := []byte(`{
		"mediaType": "application/vnd.oci.image.manifest.v1+json",
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
	if mediaType := ParseMediaType(data); mediaType != "application/vnd.oci.image.manifest.v1+json" {
		t.Fatalf("media type = %q", mediaType)
	}
}

func TestParseTopLevelAnnotations(t *testing.T) {
	data := []byte(`{"annotations":{"com.example":"yes"}}`)

	got := ParseTopLevelAnnotations(data)
	if got["com.example"] != "yes" {
		t.Fatalf("annotation = %q, want yes", got["com.example"])
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
			got := BuildObservations(tt.annotations, tt.labels, "")
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

func TestBuildObservationsForIndexMediaType(t *testing.T) {
	got := BuildObservations(
		map[string]string{"annotation": "yes"},
		nil,
		"application/vnd.oci.image.index.v1+json",
	)
	if len(got) != 2 {
		t.Fatalf("observations = %d, want 2", len(got))
	}
	if got[1].Message != "image reference resolved to an image index or manifest list" {
		t.Fatalf("message = %q", got[1].Message)
	}
	if got[1].Hint != "top-level annotations may be index-level metadata; platform-specific manifest annotations may require selecting a platform in a future version" {
		t.Fatalf("hint = %q", got[1].Hint)
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
				return []byte(`{"mediaType":"application/vnd.oci.image.manifest.v1+json","annotations":{"com.example":"yes"}}`), nil, nil
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
	if trace.Source != "crane" {
		t.Fatalf("Source = %q, want crane", trace.Source)
	}
	if trace.MediaType != "application/vnd.oci.image.manifest.v1+json" {
		t.Fatalf("MediaType = %q", trace.MediaType)
	}
	if trace.ManifestAnnotations["com.example"] != "yes" {
		t.Fatalf("annotation = %q, want yes", trace.ManifestAnnotations["com.example"])
	}
	if trace.ConfigLabels["role"] != "test" {
		t.Fatalf("label = %q, want test", trace.ConfigLabels["role"])
	}
}

func TestTraceImageUsesSkopeoWhenCraneMissing(t *testing.T) {
	restore := replaceCommandHooks(
		func(name string) (string, error) {
			if name == "skopeo" {
				return "/usr/bin/skopeo", nil
			}
			return "", errors.New("not found")
		},
		func(name string, args ...string) ([]byte, []byte, error) {
			if name != "skopeo" {
				t.Fatalf("command = %s, want skopeo", name)
			}
			switch {
			case reflect.DeepEqual(args, []string{"inspect", "--raw", "docker://example.com/test:latest"}):
				return []byte(`{"mediaType":"application/vnd.oci.image.index.v1+json","annotations":{"com.example":"yes"}}`), nil, nil
			case reflect.DeepEqual(args, []string{"inspect", "docker://example.com/test:latest"}):
				return []byte(`{"Labels":{"role":"test"}}`), nil, nil
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
	if trace.Source != "skopeo" {
		t.Fatalf("Source = %q, want skopeo", trace.Source)
	}
	if trace.MediaType != "application/vnd.oci.image.index.v1+json" {
		t.Fatalf("MediaType = %q", trace.MediaType)
	}
	if !hasObservation(trace.Observations, "image reference resolved to an image index or manifest list") {
		t.Fatalf("missing index observation: %#v", trace.Observations)
	}
}

func TestTraceImageUsesDockerWhenOnlyDockerAvailable(t *testing.T) {
	restore := replaceCommandHooks(
		func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", errors.New("not found")
		},
		func(name string, args ...string) ([]byte, []byte, error) {
			if name != "docker" {
				t.Fatalf("command = %s, want docker", name)
			}
			if !reflect.DeepEqual(args, []string{"inspect", "example.com/test:latest"}) {
				t.Fatalf("unexpected args: %v", args)
			}
			return []byte(`[{"Config":{"Labels":{"role":"test"}}}]`), nil, nil
		},
	)
	defer restore()

	trace, err := TraceImage("example.com/test:latest")
	if err != nil {
		t.Fatalf("TraceImage: %v", err)
	}
	if trace.Source != "docker" {
		t.Fatalf("Source = %q, want docker", trace.Source)
	}
	if !hasObservation(trace.Observations, "docker inspect fallback used") {
		t.Fatalf("missing docker fallback observation: %#v", trace.Observations)
	}
	for _, observation := range trace.Observations {
		if observation.Message == "docker inspect fallback used" &&
			observation.Hint != "docker inspect usually exposes local image config labels only; it usually cannot show remote manifest annotations. Install crane or skopeo for better metadata tracing." {
			t.Fatalf("docker hint = %q", observation.Hint)
		}
	}
}

func TestFormatTextIncludesSourceMediaTypeAndIndentedSections(t *testing.T) {
	trace := &MetadataTrace{
		Image:               "example.com/test:latest",
		Source:              "crane",
		MediaType:           "application/vnd.oci.image.manifest.v1+json",
		ManifestAnnotations: map[string]string{"com.example": "yes"},
		ConfigLabels:        map[string]string{"role": "test"},
		Observations: []Observation{{
			Message: "both manifest annotations and config labels present",
			Hint:    "verify propagation",
		}},
	}

	got := trace.FormatText()
	for _, want := range []string{
		"image: example.com/test:latest\nsource: crane\nmedia type: application/vnd.oci.image.manifest.v1+json\n",
		"manifest annotations:\n  com.example=yes\n",
		"config labels:\n  role=test\n",
		"observations:\n\n  - both manifest annotations and config labels present\n    hint: verify propagation\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatText() missing %q in:\n%s", want, got)
		}
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

func hasObservation(observations []Observation, message string) bool {
	for _, observation := range observations {
		if observation.Message == message {
			return true
		}
	}
	return false
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
