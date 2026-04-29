package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// TraceEntry is the on-disk record produced for one hook execution.
type TraceEntry struct {
	Timestamp    time.Time       `json:"timestamp"`
	HookStage    string          `json:"hookStage"`
	HookIndex    int             `json:"hookIndex"`
	Path         string          `json:"path"`
	Args         []string        `json:"args,omitempty"`
	Env          []string        `json:"env,omitempty"`
	Timeout      int             `json:"timeout,omitempty"`
	DurationMs   int64           `json:"durationMs"`
	ExitCode     int             `json:"exitCode"`
	Signal       string          `json:"signal,omitempty"`
	Error        string          `json:"error,omitempty"`
	TimedOut     bool            `json:"timedOut,omitempty"`
	Stdout       string          `json:"stdout,omitempty"`
	Stderr       string          `json:"stderr,omitempty"`
	State        json.RawMessage `json:"state,omitempty"`
	TraceVersion int             `json:"traceVersion"`
}

// RunWrapper executes the original hook described by args and writes a trace.
func RunWrapper(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("hook-wrapper", flag.ContinueOnError)
	fs.SetOutput(stderr)

	stage := fs.String("hook-stage", getenv("CATCHY_HOOK_STAGE", ""), "OCI hook stage")
	index := fs.Int("hook-index", getenvInt("CATCHY_HOOK_INDEX", 0), "hook index within stage")
	origPath := fs.String("orig-path", getenv("CATCHY_ORIG_PATH", ""), "original hook path")
	origArgsJSON := fs.String("orig-args-json", getenv("CATCHY_ORIG_ARGS_JSON", ""), "original hook argv as JSON")
	origEnvJSON := fs.String("orig-env-json", getenv("CATCHY_ORIG_ENV_JSON", ""), "original hook env as JSON")
	origTimeout := fs.Int("orig-timeout", getenvInt("CATCHY_ORIG_TIMEOUT", 0), "original hook timeout in seconds")
	traceDir := fs.String("trace-dir", getenv("CATCHY_TRACE_DIR", filepath.Join(os.TempDir(), "catchy-traces")), "trace output directory")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *origPath == "" {
		fmt.Fprintln(stderr, "catchy wrapper: missing --orig-path")
		return 2
	}

	origArgs, err := parseJSONArray(*origArgsJSON)
	if err != nil {
		fmt.Fprintf(stderr, "catchy wrapper: invalid --orig-args-json: %v\n", err)
		return 2
	}
	if len(origArgs) == 0 {
		origArgs = []string{*origPath}
	}

	origEnvSpecified := *origEnvJSON != ""
	origEnv, err := parseJSONArray(*origEnvJSON)
	if err != nil {
		fmt.Fprintf(stderr, "catchy wrapper: invalid --orig-env-json: %v\n", err)
		return 2
	}
	traceEnv := origEnv
	if len(origEnv) == 0 {
		origEnv = os.Environ()
		traceEnv = nil
	}

	state, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "catchy wrapper: read state: %v\n", err)
		return 2
	}

	entry := TraceEntry{
		Timestamp:    time.Now().UTC(),
		HookStage:    *stage,
		HookIndex:    *index,
		Path:         *origPath,
		Args:         origArgs,
		Env:          traceEnv,
		Timeout:      *origTimeout,
		State:        json.RawMessage(state),
		TraceVersion: 1,
	}
	if origEnvSpecified && traceEnv == nil {
		entry.Env = []string{}
	}

	ctx := context.Background()
	cancel := func() {}
	if *origTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*origTimeout)*time.Second)
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, *origPath)
	cmd.Args = origArgs
	cmd.Env = origEnv
	cmd.Stdin = bytes.NewReader(state)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(stderr, &stderrBuf)

	start := time.Now()
	err = cmd.Run()
	entry.DurationMs = time.Since(start).Milliseconds()
	entry.Stdout = stdoutBuf.String()
	entry.Stderr = stderrBuf.String()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		entry.TimedOut = true
	}

	exitCode := 0
	if err != nil {
		exitCode = 1
		entry.Error = err.Error()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				entry.Signal = status.Signal().String()
			}
		}
	}
	entry.ExitCode = exitCode

	if err := writeTrace(*traceDir, entry); err != nil {
		fmt.Fprintf(stderr, "catchy wrapper: write trace: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	return exitCode
}

func writeTrace(traceDir string, entry TraceEntry) error {
	if err := os.MkdirAll(traceDir, 0755); err != nil {
		return err
	}
	name := fmt.Sprintf("%s_%s_%d_%d.json",
		entry.Timestamp.Format("20060102T150405.000000000Z"),
		safeName(entry.HookStage),
		entry.HookIndex,
		os.Getpid(),
	)
	path := filepath.Join(traceDir, name)
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func parseJSONArray(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func getenv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func safeName(s string) string {
	if s == "" {
		return "hook"
	}
	out := []byte(s)
	for i, c := range out {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		out[i] = '_'
	}
	return string(out)
}
