package diagnose

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jiwahn/catchy/internal/report"
)

// Result is the structured diagnosis for a trace directory.
type Result struct {
	TotalTraces  int       `json:"totalTraces"`
	FailedTraces int       `json:"failedTraces"`
	Failures     []Failure `json:"failures"`
}

// Failure summarizes one failed hook execution.
type Failure struct {
	HookStage   string `json:"hookStage"`
	HookIndex   int    `json:"hookIndex"`
	Path        string `json:"path"`
	ExitCode    int    `json:"exitCode"`
	Signal      string `json:"signal,omitempty"`
	TimedOut    bool   `json:"timedOut,omitempty"`
	DurationMs  int64  `json:"durationMs"`
	Error       string `json:"error,omitempty"`
	Stderr      string `json:"stderr,omitempty"`
	Stdout      string `json:"stdout,omitempty"`
	Redacted    bool   `json:"redacted"`
	File        string `json:"file,omitempty"`
	LikelyCause string `json:"likelyCause"`
}

// ParseDir parses trace files and returns a failure-focused diagnosis.
func ParseDir(traceDir string) (*Result, error) {
	r, err := report.ParseDir(traceDir)
	if err != nil {
		return nil, err
	}
	return FromReport(r), nil
}

// FromReport builds a diagnosis from an already parsed report.
func FromReport(r *report.Report) *Result {
	result := &Result{TotalTraces: len(r.Entries)}
	for _, entry := range r.Entries {
		if !failed(entry) {
			continue
		}
		result.Failures = append(result.Failures, Failure{
			HookStage:   entry.HookStage,
			HookIndex:   entry.HookIndex,
			Path:        entry.Path,
			ExitCode:    entry.ExitCode,
			Signal:      entry.Signal,
			TimedOut:    entry.TimedOut,
			DurationMs:  entry.DurationMs,
			Error:       entry.Error,
			Stderr:      trimMultiline(entry.Stderr),
			Stdout:      trimMultiline(entry.Stdout),
			Redacted:    entry.Redacted,
			File:        entry.File,
			LikelyCause: likelyCause(entry),
		})
	}
	result.FailedTraces = len(result.Failures)
	return result
}

// FormatText returns a concise human-readable diagnosis.
func (r *Result) FormatText() string {
	if r.TotalTraces == 0 {
		return "no hook traces found\n"
	}
	if r.FailedTraces == 0 {
		return fmt.Sprintf("no hook failures found\ntraces: %d\n", r.TotalTraces)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "hook failures: %d of %d traces\n", r.FailedTraces, r.TotalTraces)
	for i, failure := range r.Failures {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s[%d] failed\n", failure.HookStage, failure.HookIndex)
		fmt.Fprintf(&b, "path: %s\n", failure.Path)
		fmt.Fprintf(&b, "exit: %d\n", failure.ExitCode)
		if failure.Signal != "" {
			fmt.Fprintf(&b, "signal: %s\n", failure.Signal)
		}
		if failure.TimedOut {
			b.WriteString("timed out: true\n")
		}
		fmt.Fprintf(&b, "duration: %dms\n", failure.DurationMs)
		if failure.Redacted {
			b.WriteString("redacted: true\n")
		}
		fmt.Fprintf(&b, "likely cause: %s\n", failure.LikelyCause)
		if failure.Error != "" {
			fmt.Fprintf(&b, "error: %s\n", failure.Error)
		}
		if failure.Stderr != "" {
			fmt.Fprintf(&b, "stderr: %s\n", failure.Stderr)
		}
		if failure.Stdout != "" {
			fmt.Fprintf(&b, "stdout: %s\n", failure.Stdout)
		}
		if failure.File != "" {
			fmt.Fprintf(&b, "trace: %s\n", failure.File)
		}
	}
	return b.String()
}

// FormatJSON returns a machine-readable diagnosis.
func (r *Result) FormatJSON() string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data) + "\n"
}

func failed(entry report.Entry) bool {
	return entry.ExitCode != 0 || entry.Signal != "" || entry.TimedOut || entry.Error != ""
}

func likelyCause(entry report.Entry) string {
	switch {
	case entry.TimedOut:
		return "hook timed out"
	case entry.Signal != "":
		return "hook terminated by signal"
	case entry.ExitCode != 0:
		return "hook exited with non-zero status"
	case entry.Error != "":
		return "wrapper reported an execution error"
	default:
		return "hook failure detected"
	}
}

func trimMultiline(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > 240 {
		return s[:240] + "..."
	}
	return s
}
