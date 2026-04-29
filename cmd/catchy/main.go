package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"catchy/internal/hook"
	"catchy/internal/report"
	"catchy/internal/spec"
)

// version of catchy (update when releasing)
const version = "0.0.1"

// printUsage prints a basic usage message.
func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: catchy <command> [options]

Commands:
    inspect   Inspect an OCI bundle and list its hooks
    wrap      Rewrite hooks in a bundle to wrap them with a trace wrapper
    restore   Restore config.json from config.json.catchy.bak
    run       Wrap hooks and run the container via an OCI runtime
    report    Summarise collected hook trace logs

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
	case "wrap":
		wrapCmd(os.Args[2:])
	case "restore":
		restoreCmd(os.Args[2:])
	case "run":
		runCmd(os.Args[2:])
	case "report":
		reportCmd(os.Args[2:])
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

// inspectCmd parses flags and calls the inspect subcommand.
func inspectCmd(args []string) {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy inspect <bundle>\n\n")
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

	printHooks("prestart", b.Hooks.Prestart)
	printHooks("createRuntime", b.Hooks.CreateRuntime)
	printHooks("createContainer", b.Hooks.CreateContainer)
	printHooks("startContainer", b.Hooks.StartContainer)
	printHooks("poststart", b.Hooks.Poststart)
	printHooks("poststop", b.Hooks.Poststop)

}

// wrapCmd rewrites hooks in the bundle.
func wrapCmd(args []string) {
	fs := flag.NewFlagSet("wrap", flag.ExitOnError)
	defaultWrapper, _ := os.Executable()
	wrapperPath := fs.String("wrapper", defaultWrapper, "path to the catchy wrapper executable")
	traceDir := fs.String("trace-dir", "", "directory for hook trace JSON files (default: <bundle>/.catchy/traces)")
	force := fs.Bool("force", false, "overwrite an existing config.json.catchy.bak backup")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy wrap [--wrapper /path/to/catchy] [--trace-dir DIR] [--force] <bundle>\n\n")
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
	if err := hook.WrapBundleWithOptions(bundle, *wrapperPath, hook.WrapOptions{Force: *force, TraceDir: *traceDir}); err != nil {
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
	runtimeArgs := fs.String("runtime-args", "", "extra arguments passed before runtime run, split on whitespace")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: catchy run [--runtime runc] [--wrapper /path/to/catchy] [--trace-dir DIR] <bundle>\n\n")
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
	err := hook.WrapBundleWithOptions(bundle, *wrapperPath, hook.WrapOptions{Force: true, TraceDir: *traceDir})
	if err != nil && !errors.Is(err, hook.ErrNoHooks) {
		fmt.Fprintf(os.Stderr, "failed to wrap bundle: %v\n", err)
		os.Exit(1)
	}
	if err == nil {
		wrapped = true
	}
	if wrapped && !*keepWrapped {
		defer func() {
			if err := hook.RestoreBundle(bundle); err != nil {
				fmt.Fprintf(os.Stderr, "failed to restore bundle: %v\n", err)
			}
		}()
	}

	cmdArgs := append(splitArgs(*runtimeArgs), "run", "-b", bundle, *id)
	cmd := exec.Command(*runtime, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
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

func printHooks(name string, hooks []spec.Hook) {
	if len(hooks) == 0 {
		return
	}

	fmt.Printf("%s:\n", name)
	for i, h := range hooks {
		fmt.Printf("  [%d]\n", i)
		fmt.Printf("    path: %s\n", h.Path)

		if len(h.Args) > 0 {
			fmt.Printf("    args: %v\n", h.Args)
		}

		if len(h.Env) > 0 {
			fmt.Printf("    env: %v\n", h.Env)
		}

		if h.Timeout > 0 {
			fmt.Printf("    timeout: %d\n", h.Timeout)
		}
	}
}

func splitArgs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Fields(raw)
}
