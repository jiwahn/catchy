package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jiwahn/catchy/internal/check"
	"github.com/jiwahn/catchy/internal/diagnose"
	"github.com/jiwahn/catchy/internal/hook"
	"github.com/jiwahn/catchy/internal/report"
	"github.com/jiwahn/catchy/internal/spec"
)

// version of catchy (update when releasing)
const version = "0.0.1"

// printUsage prints a basic usage message.
func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: catchy <command> [options]

Commands:
    inspect   Inspect an OCI bundle and list its hooks
    check     Preflight validate OCI hook definitions
    wrap      Rewrite hooks in a bundle to wrap them with a trace wrapper
    restore   Restore config.json from config.json.catchy.bak
    run       Wrap hooks and run the container via an OCI runtime
    report    Summarise collected hook trace logs
    diagnose  Summarise hook failures from collected traces

Use "catchy <command> -h" for more information about a command.
`)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	switch cmd {
	case "inspect":
		inspectCmd(os.Args[2:])
	case "check":
		checkCmd(os.Args[2:])
	case "wrap":
		wrapCmd(os.Args[2:])
	case "restore":
		restoreCmd(os.Args[2:])
	case "run":
		runCmd(os.Args[2:])
	case "report":
		reportCmd(os.Args[2:])
	case "diagnose":
		diagnoseCmd(os.Args[2:])
	case "hook-wrapper":
		os.Exit(hook.RunWrapper(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	case "version":
		fmt.Println(version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// checkCmd validates OCI hook definitions before runtime execution.
func checkCmd(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	format := fs.String("format", "text", "output format: text, json")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy check [--format text] <bundle>\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	result, err := check.CheckBundle(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to check bundle: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "text":
		fmt.Print(result.FormatText())
	case "json":
		fmt.Print(result.FormatJSON())
	default:
		fmt.Fprintf(os.Stderr, "unknown check format: %s\n", *format)
		os.Exit(1)
	}
	if result.HasProblems() {
		os.Exit(1)
	}
}

// inspectCmd parses flags and calls the inspect subcommand.
func inspectCmd(args []string) {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	noRedact := fs.Bool("no-redact", false, "disable inspect output redaction")
	var redactKeys stringListFlag
	fs.Var(&redactKeys, "redact-key", "additional sensitive key pattern for inspect redaction; may be repeated")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy inspect [--no-redact] [--redact-key KEY] <bundle>\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	bundle := fs.Arg(0)
	cfgPath := filepath.Join(bundle, "config.json")

	b, err := spec.LoadBundle(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load bundle: %v\n", err)
		os.Exit(1)
	}

	if b.Hooks == nil {
		fmt.Println("no hooks found")
		return
	}

	redaction := hook.NewRedactionConfig(!*noRedact, redactKeys)
	printHooks(os.Stdout, "prestart", b.Hooks.Prestart, redaction)
	printHooks(os.Stdout, "createRuntime", b.Hooks.CreateRuntime, redaction)
	printHooks(os.Stdout, "createContainer", b.Hooks.CreateContainer, redaction)
	printHooks(os.Stdout, "startContainer", b.Hooks.StartContainer, redaction)
	printHooks(os.Stdout, "poststart", b.Hooks.Poststart, redaction)
	printHooks(os.Stdout, "poststop", b.Hooks.Poststop, redaction)

}

// wrapCmd rewrites hooks in the bundle.
func wrapCmd(args []string) {
	fs := flag.NewFlagSet("wrap", flag.ExitOnError)
	defaultWrapper, _ := os.Executable()
	wrapperPath := fs.String("wrapper", defaultWrapper, "path to the catchy wrapper executable")
	traceDir := fs.String("trace-dir", "", "directory for hook trace JSON files (default: <bundle>/.catchy/traces)")
	force := fs.Bool("force", false, "overwrite an existing config.json.catchy.bak backup")
	noRedact := fs.Bool("no-redact", false, "disable trace redaction")
	var redactKeys stringListFlag
	fs.Var(&redactKeys, "redact-key", "additional sensitive key pattern for trace redaction; may be repeated")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy wrap [--wrapper /path/to/catchy] [--trace-dir DIR] [--force] [--no-redact] [--redact-key KEY] <bundle>\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	bundle := fs.Arg(0)
	if err := hook.WrapBundleWithOptions(bundle, *wrapperPath, hook.WrapOptions{
		Force:      *force,
		TraceDir:   *traceDir,
		NoRedact:   *noRedact,
		RedactKeys: redactKeys,
	}); err != nil {
		if errors.Is(err, hook.ErrNoHooks) {
			fmt.Println("no hooks found")
			return
		}
		fmt.Fprintf(os.Stderr, "failed to wrap bundle: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrapped hooks in bundle %s\n", bundle)
}

// restoreCmd restores config.json from config.json.catchy.bak.
func restoreCmd(args []string) {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy restore <bundle>\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	bundle := fs.Arg(0)
	if err := hook.RestoreBundle(bundle); err != nil {
		fmt.Fprintf(os.Stderr, "failed to restore bundle: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("restored bundle %s\n", bundle)
}

// runCmd wraps hooks and runs the runtime.
func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	runtime := fs.String("runtime", "runc", "OCI runtime to use (runc, crun, etc.)")
	defaultWrapper, _ := os.Executable()
	wrapperPath := fs.String("wrapper", defaultWrapper, "path to the catchy wrapper executable")
	traceDir := fs.String("trace-dir", "", "directory for hook trace JSON files (default: <bundle>/.catchy/traces)")
	id := fs.String("id", "", "container id to pass to the runtime")
	keepWrapped := fs.Bool("keep-wrapped", false, "leave config.json wrapped after runtime exits")
	runtimeArgs := fs.String("runtime-args", "", "legacy extra arguments passed before runtime run, split on whitespace")
	var runtimeArgList stringListFlag
	fs.Var(&runtimeArgList, "runtime-arg", "extra argument passed before runtime run; may be repeated")
	noRedact := fs.Bool("no-redact", false, "disable trace redaction")
	var redactKeys stringListFlag
	fs.Var(&redactKeys, "redact-key", "additional sensitive key pattern for trace redaction; may be repeated")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy run [--runtime runc] [--wrapper /path/to/catchy] [--trace-dir DIR] [--no-redact] [--redact-key KEY] <bundle>\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	bundle := fs.Arg(0)
	if *id == "" {
		*id = fmt.Sprintf("catchy-%d", os.Getpid())
	}

	wrapped := false
	err := hook.WrapBundleWithOptions(bundle, *wrapperPath, hook.WrapOptions{
		Force:      true,
		TraceDir:   *traceDir,
		NoRedact:   *noRedact,
		RedactKeys: redactKeys,
	})
	if err != nil && !errors.Is(err, hook.ErrNoHooks) {
		fmt.Fprintf(os.Stderr, "failed to wrap bundle: %v\n", err)
		os.Exit(1)
	}
	if err == nil {
		wrapped = true
	}
	restored := false
	if wrapped && !*keepWrapped {
		defer func() {
			if !restored {
				restoreAfterRun(bundle)
			}
		}()
	}

	cmdArgs := buildRuntimeCommandArgs(*runtimeArgs, runtimeArgList, bundle, *id)
	cmd := exec.Command(*runtime, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if wrapped && !*keepWrapped {
			restoreAfterRun(bundle)
			restored = true
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "failed to run runtime: %v\n", err)
		os.Exit(1)
	}
}

// reportCmd summarises collected hook logs.
func reportCmd(args []string) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	format := fs.String("format", "text", "output format: text, json, yaml")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy report [--format text] <trace-dir>\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	traceDir := fs.Arg(0)
	r, err := report.ParseDir(traceDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse traces: %v\n", err)
		os.Exit(1)
	}
	switch *format {
	case "text":
		fmt.Print(r.FormatText())
	case "json":
		fmt.Print(r.FormatJSON())
	case "yaml":
		fmt.Print(r.FormatYAML())
	default:
		fmt.Fprintf(os.Stderr, "unknown report format: %s\n", *format)
		os.Exit(1)
	}
}

// diagnoseCmd prints a failure-focused summary of hook traces.
func diagnoseCmd(args []string) {
	fs := flag.NewFlagSet("diagnose", flag.ExitOnError)
	format := fs.String("format", "text", "output format: text, json")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy diagnose [--format text] <trace-dir>\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	traceDir := fs.Arg(0)
	result, err := diagnose.ParseDir(traceDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to diagnose traces: %v\n", err)
		os.Exit(1)
	}
	switch *format {
	case "text":
		fmt.Print(result.FormatText())
	case "json":
		fmt.Print(result.FormatJSON())
	default:
		fmt.Fprintf(os.Stderr, "unknown diagnose format: %s\n", *format)
		os.Exit(1)
	}
}

func printHooks(w io.Writer, name string, hooks []spec.Hook, redaction hook.RedactionConfig) {
	if len(hooks) == 0 {
		return
	}

	fmt.Fprintf(w, "%s:\n", name)
	for i, h := range hooks {
		fmt.Fprintf(w, "  [%d]\n", i)
		fmt.Fprintf(w, "    path: %s\n", h.Path)

		if len(h.Args) > 0 {
			fmt.Fprintf(w, "    args: %v\n", hook.RedactStringSlice(h.Args, redaction))
		}

		if len(h.Env) > 0 {
			fmt.Fprintf(w, "    env: %v\n", hook.RedactEnv(h.Env, redaction))
		}

		if h.Timeout > 0 {
			fmt.Fprintf(w, "    timeout: %d\n", h.Timeout)
		}
	}
}

func splitArgs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Fields(raw)
}

func buildRuntimeCommandArgs(runtimeArgs string, runtimeArgList []string, bundle string, id string) []string {
	cmdArgs := append([]string{}, splitArgs(runtimeArgs)...)
	cmdArgs = append(cmdArgs, runtimeArgList...)
	cmdArgs = append(cmdArgs, "run", "-b", bundle, id)
	return cmdArgs
}

func restoreAfterRun(bundle string) {
	if err := hook.RestoreBundle(bundle); err != nil {
		fmt.Fprintf(os.Stderr, "failed to restore bundle: %v\n", err)
	}
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}
