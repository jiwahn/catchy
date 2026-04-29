package report

// Package report parses collected hook trace logs and
// summarises them into human or machine readable formats.

// This file defines stub structures used by the CLI.

// Entry represents a single hook execution trace.  A real
// implementation would include fields such as timestamp,
// hook type, command, args, exit code, duration, stdout,
// stderr and the state JSON.
type Entry struct {
    HookType string
    Path     string
    ExitCode int
    DurationMs int64
    Stdout   string
    Stderr   string
}

// Report summarises a collection of entries.
type Report struct {
    Entries []Entry
}

// ParseDir parses a directory containing hook trace logs and returns
// a report.  This stub returns an empty report.
func ParseDir(dir string) (*Report, error) {
    // TODO: read files under dir, parse each entry into the Report
    return &Report{Entries: nil}, nil
}

// FormatText formats the report as a human readable string.
func (r *Report) FormatText() string {
    // TODO: implement actual formatting.  For now return a placeholder.
    return "report formatting not implemented"
}

// FormatJSON formats the report as JSON.
func (r *Report) FormatJSON() string {
    // TODO: implement JSON formatting
    return "{}"
}

// FormatYAML formats the report as YAML.
func (r *Report) FormatYAML() string {
    // TODO: implement YAML formatting
    return "--- {}"
}