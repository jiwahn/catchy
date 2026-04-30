package metadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// MetadataTrace describes image metadata found through external CLI tools.
type MetadataTrace struct {
	Image               string            `json:"image"`
	Source              string            `json:"source,omitempty"`
	MediaType           string            `json:"mediaType,omitempty"`
	ManifestAnnotations map[string]string `json:"manifestAnnotations,omitempty"`
	ConfigLabels        map[string]string `json:"configLabels,omitempty"`
	Observations        []Observation     `json:"observations,omitempty"`
}

// Observation explains what the metadata shape likely means.
type Observation struct {
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type commandRunner func(name string, args ...string) ([]byte, []byte, error)

var (
	lookPath               = exec.LookPath
	runCmd   commandRunner = runExternalCommand
)

// TraceImage reads image metadata using crane, skopeo, or docker.
func TraceImage(image string) (*MetadataTrace, error) {
	if strings.TrimSpace(image) == "" {
		return nil, errors.New("image is required")
	}
	trace := &MetadataTrace{Image: image}
	var extraObservations []Observation

	switch {
	case toolAvailable("crane"):
		trace.Source = "crane"
		if err := traceWithCrane(trace, image); err != nil {
			return nil, err
		}
	case toolAvailable("skopeo"):
		trace.Source = "skopeo"
		if err := traceWithSkopeo(trace, image); err != nil {
			return nil, err
		}
	case toolAvailable("docker"):
		trace.Source = "docker"
		if err := traceWithDocker(trace, image); err != nil {
			return nil, err
		}
		extraObservations = append(extraObservations, Observation{
			Message: "docker inspect fallback used",
			Hint:    "docker inspect usually exposes local image config labels only; it usually cannot show remote manifest annotations. Install crane or skopeo for better metadata tracing.",
		})
	default:
		return nil, errors.New("no supported tool found (crane, skopeo, docker)\nhint: install crane (recommended) or skopeo to enable metadata tracing")
	}

	trace.Observations = BuildObservations(trace.ManifestAnnotations, trace.ConfigLabels, trace.MediaType)
	trace.Observations = append(trace.Observations, extraObservations...)
	return trace, nil
}

// FormatText returns a concise human-readable metadata trace.
func (t *MetadataTrace) FormatText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "image: %s\n", t.Image)
	if t.Source != "" {
		fmt.Fprintf(&b, "source: %s\n", t.Source)
	}
	if t.MediaType != "" {
		fmt.Fprintf(&b, "media type: %s\n", t.MediaType)
	}
	writeStringMap(&b, "manifest annotations", t.ManifestAnnotations)
	writeStringMap(&b, "config labels", t.ConfigLabels)
	if len(t.Observations) > 0 {
		b.WriteString("\nobservations:\n")
		for _, observation := range t.Observations {
			fmt.Fprintf(&b, "\n  - %s\n", observation.Message)
			if observation.Hint != "" {
				fmt.Fprintf(&b, "    hint: %s\n", observation.Hint)
			}
		}
	}
	return b.String()
}

// FormatJSON returns a machine-readable metadata trace.
func (t *MetadataTrace) FormatJSON() string {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data) + "\n"
}

// BuildObservations explains likely metadata propagation behavior.
func BuildObservations(annotations map[string]string, labels map[string]string, mediaType string) []Observation {
	hasAnnotations := len(annotations) > 0
	hasLabels := len(labels) > 0
	var observations []Observation

	switch {
	case hasAnnotations && !hasLabels:
		observations = append(observations, Observation{
			Message: "manifest annotations present but not reflected in config labels",
			Hint:    "many runtimes (including containerd) do not propagate manifest annotations into runtime configuration",
		})
	case hasLabels && !hasAnnotations:
		observations = append(observations, Observation{
			Message: "config labels present",
			Hint:    "labels are typically available at container runtime level, unlike manifest annotations",
		})
	case !hasAnnotations && !hasLabels:
		observations = append(observations, Observation{
			Message: "no annotations or labels found in image metadata",
			Hint:    "image may not include custom metadata; hook-based features relying on annotations will not work",
		})
	default:
		observations = append(observations, Observation{
			Message: "both manifest annotations and config labels present",
			Hint:    "verify which fields your runtime actually propagates (containerd typically uses config labels, not manifest annotations)",
		})
	}
	if isIndexMediaType(mediaType) {
		observations = append(observations, Observation{
			Message: "image reference resolved to an image index or manifest list",
			Hint:    "top-level annotations may be index-level metadata; platform-specific manifest annotations may require selecting a platform in a future version",
		})
	}
	return observations
}

func traceWithCrane(trace *MetadataTrace, image string) error {
	manifest, stderr, err := runCmd("crane", "manifest", image)
	if err != nil {
		return commandError("crane manifest", stderr, err)
	}
	config, stderr, err := runCmd("crane", "config", image)
	if err != nil {
		return commandError("crane config", stderr, err)
	}
	trace.MediaType = ParseMediaType(manifest)
	trace.ManifestAnnotations = ParseTopLevelAnnotations(manifest)
	trace.ConfigLabels = ParseConfigLabels(config)
	return nil
}

func traceWithSkopeo(trace *MetadataTrace, image string) error {
	ref := "docker://" + image
	manifest, stderr, err := runCmd("skopeo", "inspect", "--raw", ref)
	if err != nil {
		return commandError("skopeo inspect --raw", stderr, err)
	}
	inspect, stderr, err := runCmd("skopeo", "inspect", ref)
	if err != nil {
		return commandError("skopeo inspect", stderr, err)
	}
	trace.MediaType = ParseMediaType(manifest)
	trace.ManifestAnnotations = ParseTopLevelAnnotations(manifest)
	trace.ConfigLabels = parseSkopeoLabels(inspect)
	return nil
}

func traceWithDocker(trace *MetadataTrace, image string) error {
	out, stderr, err := runCmd("docker", "inspect", image)
	if err != nil {
		return commandError("docker inspect", stderr, err)
	}
	trace.ConfigLabels = parseDockerLabels(out)
	return nil
}

// ParseTopLevelAnnotations extracts top-level annotations from manifest JSON.
func ParseTopLevelAnnotations(data []byte) map[string]string {
	var manifest struct {
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	return cleanMap(manifest.Annotations)
}

// ParseManifestAnnotations extracts top-level manifest annotations.
func ParseManifestAnnotations(data []byte) map[string]string {
	return ParseTopLevelAnnotations(data)
}

// ParseMediaType extracts mediaType from manifest JSON.
func ParseMediaType(data []byte) string {
	var manifest struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}
	return manifest.MediaType
}

// ParseConfigLabels extracts config.Labels from an image config JSON document.
func ParseConfigLabels(data []byte) map[string]string {
	var config struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	return cleanMap(config.Config.Labels)
}

func parseSkopeoLabels(data []byte) map[string]string {
	var inspect struct {
		Labels map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(data, &inspect); err != nil {
		return nil
	}
	return cleanMap(inspect.Labels)
}

func parseDockerLabels(data []byte) map[string]string {
	var inspect []struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(data, &inspect); err != nil || len(inspect) == 0 {
		return nil
	}
	return cleanMap(inspect[0].Config.Labels)
}

func toolAvailable(name string) bool {
	_, err := lookPath(name)
	return err == nil
}

func runExternalCommand(name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return out, stderr.Bytes(), err
}

func commandError(command string, stderr []byte, err error) error {
	message := strings.TrimSpace(string(stderr))
	if message == "" {
		message = err.Error()
	}
	return fmt.Errorf("%s failed: %s", command, message)
}

func cleanMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func writeStringMap(b *strings.Builder, title string, values map[string]string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(b, "  %s=%s\n", key, values[key])
	}
}

func isIndexMediaType(mediaType string) bool {
	switch mediaType {
	case "application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json":
		return true
	default:
		return false
	}
}
