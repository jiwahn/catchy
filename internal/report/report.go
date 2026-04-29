package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry represents a single hook execution trace.
type Entry struct {
	Timestamp     time.Time       `json:"timestamp"`
	HookStage     string          `json:"hookStage"`
	HookIndex     int             `json:"hookIndex"`
	Path          string          `json:"path"`
	Args          []string        `json:"args,omitempty"`
	Env           []string        `json:"env,omitempty"`
	Timeout       int             `json:"timeout,omitempty"`
	DurationMs    int64           `json:"durationMs"`
	ExitCode      int             `json:"exitCode"`
	Signal        string          `json:"signal,omitempty"`
	Error         string          `json:"error,omitempty"`
	TimedOut      bool            `json:"timedOut,omitempty"`
	Stdout        string          `json:"stdout,omitempty"`
	Stderr        string          `json:"stderr,omitempty"`
	State         json.RawMessage `json:"state,omitempty"`
	Redacted      bool            `json:"redacted"`
	RedactionKeys []string        `json:"redactionKeys,omitempty"`
	TraceVersion  int             `json:"traceVersion"`
	File          string          `json:"file,omitempty"`
}

// Report summarises a collection of entries.
type Report struct {
	Entries []Entry `json:"entries"`
}

// ParseDir parses JSON trace files under dir and returns a report.
func ParseDir(dir string) (*Report, error) {
	var entries []Entry
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		entry.File = path
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
	return &Report{Entries: entries}, nil
}

// FormatText formats the report as a human-readable summary.
func (r *Report) FormatText() string {
	if len(r.Entries) == 0 {
		return "no hook traces found\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "hook traces: %d\n", len(r.Entries))
	for _, e := range r.Entries {
		status := "ok"
		if e.ExitCode != 0 {
			status = "failed"
		}
		fmt.Fprintf(&b, "\n%s[%d] %s\n", e.HookStage, e.HookIndex, status)
		fmt.Fprintf(&b, "  path: %s\n", e.Path)
		fmt.Fprintf(&b, "  exit: %d\n", e.ExitCode)
		if e.Signal != "" {
			fmt.Fprintf(&b, "  signal: %s\n", e.Signal)
		}
		if e.TimedOut {
			fmt.Fprintf(&b, "  timed out: true\n")
		}
		fmt.Fprintf(&b, "  duration: %dms\n", e.DurationMs)
		if !e.Timestamp.IsZero() {
			fmt.Fprintf(&b, "  timestamp: %s\n", e.Timestamp.Format(time.RFC3339Nano))
		}
		if e.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", e.Error)
		}
		if e.Redacted {
			fmt.Fprintf(&b, "  redacted: true\n")
		}
		if e.Stdout != "" {
			fmt.Fprintf(&b, "  stdout: %s\n", trimMultiline(e.Stdout))
		}
		if e.Stderr != "" {
			fmt.Fprintf(&b, "  stderr: %s\n", trimMultiline(e.Stderr))
		}
	}
	return b.String()
}

// FormatJSON formats the report as JSON.
func (r *Report) FormatJSON() string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data) + "\n"
}

// FormatYAML formats the report as simple YAML without external dependencies.
func (r *Report) FormatYAML() string {
	if len(r.Entries) == 0 {
		return "entries: []\n"
	}
	var b strings.Builder
	b.WriteString("entries:\n")
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "  - timestamp: %q\n", e.Timestamp.Format(time.RFC3339Nano))
		fmt.Fprintf(&b, "    hookStage: %q\n", e.HookStage)
		fmt.Fprintf(&b, "    hookIndex: %d\n", e.HookIndex)
		fmt.Fprintf(&b, "    path: %q\n", e.Path)
		fmt.Fprintf(&b, "    exitCode: %d\n", e.ExitCode)
		fmt.Fprintf(&b, "    durationMs: %d\n", e.DurationMs)
		if e.Error != "" {
			fmt.Fprintf(&b, "    error: %q\n", e.Error)
		}
		if e.Signal != "" {
			fmt.Fprintf(&b, "    signal: %q\n", e.Signal)
		}
		if e.TimedOut {
			b.WriteString("    timedOut: true\n")
		}
		if e.Redacted {
			b.WriteString("    redacted: true\n")
		}
		if len(e.RedactionKeys) > 0 {
			b.WriteString("    redactionKeys:\n")
			for _, key := range e.RedactionKeys {
				fmt.Fprintf(&b, "      - %q\n", key)
			}
		}
	}
	return b.String()
}

func trimMultiline(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > 240 {
		return s[:240] + "..."
	}
	return s
}
