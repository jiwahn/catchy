package check

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiwahn/catchy/internal/spec"
)

const largeTimeoutSeconds = 3600

// Result is the structured preflight result for an OCI bundle.
type Result struct {
	BundlePath string      `json:"bundlePath"`
	ConfigPath string      `json:"configPath"`
	HasHooks   bool        `json:"hasHooks"`
	Checks     []HookCheck `json:"checks"`
}

// HookCheck describes preflight validation for one OCI hook.
type HookCheck struct {
	Stage    string    `json:"stage"`
	Index    int       `json:"index"`
	Path     string    `json:"path"`
	Args     []string  `json:"args,omitempty"`
	Timeout  int       `json:"timeout,omitempty"`
	Problems []Problem `json:"problems,omitempty"`
	Warnings []Problem `json:"warnings,omitempty"`
}

// Problem describes an actionable validation problem or warning.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// CheckBundle validates OCI hook definitions in bundlePath/config.json.
func CheckBundle(bundlePath string) (*Result, error) {
	configPath := filepath.Join(bundlePath, "config.json")
	bundle, err := spec.LoadBundle(configPath)
	if err != nil {
		return nil, err
	}

	result := &Result{
		BundlePath: bundlePath,
		ConfigPath: configPath,
	}
	if bundle.Hooks == nil {
		return result, nil
	}

	addChecks(result, "prestart", bundle.Hooks.Prestart)
	addChecks(result, "createRuntime", bundle.Hooks.CreateRuntime)
	addChecks(result, "createContainer", bundle.Hooks.CreateContainer)
	addChecks(result, "startContainer", bundle.Hooks.StartContainer)
	addChecks(result, "poststart", bundle.Hooks.Poststart)
	addChecks(result, "poststop", bundle.Hooks.Poststop)
	result.HasHooks = len(result.Checks) > 0
	return result, nil
}

// HasProblems reports whether any hook check found a validation problem.
func (r *Result) HasProblems() bool {
	for _, check := range r.Checks {
		if len(check.Problems) > 0 {
			return true
		}
	}
	return false
}

// FormatText returns a concise human-readable preflight result.
func (r *Result) FormatText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "bundle: %s\n", r.BundlePath)
	fmt.Fprintf(&b, "config: %s\n", r.ConfigPath)
	if !r.HasHooks {
		b.WriteString("hooks: none\n")
		return b.String()
	}

	fmt.Fprintf(&b, "hooks: %d\n", len(r.Checks))
	for _, check := range r.Checks {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "%s[%d]\n", check.Stage, check.Index)
		fmt.Fprintf(&b, "path: %s\n", check.Path)
		if len(check.Problems) > 0 {
			b.WriteString("status: failed\n")
		} else {
			b.WriteString("status: ok\n")
		}
		if len(check.Problems) > 0 {
			writeProblems(&b, "problems", check.Problems)
		}
		if len(check.Warnings) > 0 {
			writeProblems(&b, "warnings", check.Warnings)
		}
	}
	return b.String()
}

// FormatJSON returns a machine-readable preflight result.
func (r *Result) FormatJSON() string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data) + "\n"
}

func addChecks(result *Result, stage string, hooks []spec.Hook) {
	for i, hook := range hooks {
		result.Checks = append(result.Checks, validateHook(stage, i, hook))
	}
}

func validateHook(stage string, index int, hook spec.Hook) HookCheck {
	check := HookCheck{
		Stage:   stage,
		Index:   index,
		Path:    hook.Path,
		Args:    append([]string(nil), hook.Args...),
		Timeout: hook.Timeout,
	}

	if hook.Path == "" {
		check.Problems = append(check.Problems, problem("empty_path", "hook path is empty", "set an absolute host path for the hook executable"))
	} else if !filepath.IsAbs(hook.Path) {
		check.Problems = append(check.Problems, problem("non_absolute_path", "hook path is not absolute", "OCI hook paths are resolved on the host; use an absolute path"))
	} else {
		validateHookPath(&check, hook.Path)
	}

	if hook.Timeout < 0 {
		check.Problems = append(check.Problems, problem("negative_timeout", "hook timeout is negative", "use a positive timeout value or omit the timeout"))
	} else if hook.Timeout > largeTimeoutSeconds {
		check.Warnings = append(check.Warnings, problem("large_timeout", "hook timeout is unusually large", "check whether such a long hook timeout is intentional"))
	}

	return check
}

func validateHookPath(check *HookCheck, path string) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			check.Problems = append(check.Problems, problem("path_missing", "hook path does not exist on the host", "verify the hook executable exists on the host, not only inside the container rootfs"))
			return
		}
		check.Problems = append(check.Problems, problem("path_missing", "hook path does not exist on the host", "verify the hook executable exists on the host, not only inside the container rootfs"))
		return
	}
	if info.IsDir() {
		check.Problems = append(check.Problems, problem("path_is_directory", "hook path is a directory", "hook path must point to an executable file"))
		return
	}
	if info.Mode()&0111 == 0 {
		check.Problems = append(check.Problems, problem("not_executable", "hook path is not executable", fmt.Sprintf("add executable permission, for example chmod +x %s", path)))
	}
	validateShebang(check, path)
}

func validateShebang(check *HookCheck, path string) {
	line, ok := firstLine(path)
	if !ok || !strings.HasPrefix(line, "#!") {
		return
	}
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "#!")))
	if len(fields) == 0 {
		return
	}
	interpreter := fields[0]
	if !filepath.IsAbs(interpreter) {
		return
	}
	if _, err := os.Stat(interpreter); err != nil {
		check.Problems = append(check.Problems, problem("interpreter_missing", "hook interpreter does not exist on the host", "install the interpreter or update the shebang to a host-available interpreter"))
	}
}

func firstLine(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return "", false
	}
	if n == 0 {
		return "", false
	}
	data := buf[:n]
	line := string(data)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimRight(line, "\r"), true
}

func problem(code, message, hint string) Problem {
	return Problem{Code: code, Message: message, Hint: hint}
}

func writeProblems(b *strings.Builder, label string, problems []Problem) {
	fmt.Fprintf(b, "%s:\n", label)
	for _, problem := range problems {
		fmt.Fprintf(b, "- code: %s\n", problem.Code)
		fmt.Fprintf(b, "  message: %s\n", problem.Message)
		if problem.Hint != "" {
			fmt.Fprintf(b, "  hint: %s\n", problem.Hint)
		}
	}
}
