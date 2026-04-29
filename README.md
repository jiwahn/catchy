# catchy

`catchy` is a lightweight observability and debugging toolkit for **OCI runtime hooks**.  
It helps container developers and operators inspect, trace and diagnose hook execution failures across OCI‑compatible runtimes such as `runc`, `crun`, and `youki`.

## Motivation

OCI hooks execute arbitrary commands at well‑defined points during the container lifecycle.  
According to the runtime specification, prestart, createRuntime, createContainer and startContainer hooks **must be invoked by the runtime** and if any of them fails the runtime **must generate an error and stop the container**【724359104618359†L86-L98】.  
However, the spec does not mandate how the error is surfaced to the user.  In practice, containerd emits a generic error such as:

```
OCI runtime create failed: runc create failed: unable to start container process: error running hook #0: error running hook: signal: illegal instruction (core dumped), stdout: , stderr:: unknown
```

The containerd issue **"Make it POSSIBLE to debug cdi hooks"** complains that there is no way to see which hook ran, what arguments and environment it received, or why it failed【790362019473417†L232-L249】.  While `crun` exposes annotations (`run.oci.hooks.stdout`/`run.oci.hooks.stderr`) to redirect hook output to files【424383501769946†L440-L450】, this is runtime‑specific and not available in runc or containerd.

`catchy` addresses this gap by providing a cross‑runtime way to observe and debug OCI hooks **without patching the runtime**.

## Features

* **Inspect** a bundle’s `config.json` and summarise its hooks (type, path, args, env, timeout).  
* **Wrap** existing hooks with a thin wrapper that captures stdout, stderr, exit status, duration and the state JSON passed to the hook.  
* **Run** a container using a chosen OCI runtime (`runc`, `crun`, etc.) while automatically wrapping its hooks and collecting traces.  
* **Report** hook execution traces in human‑readable or machine‑readable formats (text, JSON, YAML).  
* Designed as an external CLI; no need to modify containerd or the runtime.

## Before / After

OCI hook failures are often opaque. A normal runtime run may only say that a hook failed, without preserving enough detail to quickly identify which command ran, what it printed, or what state JSON it received.

With the failing-hook demo, a direct runtime run looks like this:

```
$ runc run -b examples/failing-hook/.work/bundle catchy-demo-direct
error running prestart hook #0: exit status 42
```

The hook printed a useful diagnostic, but depending on the runtime and caller that output may be missing, truncated, or buried in a larger error.

Running the same bundle through `catchy` still fails the container, but it leaves a trace:

```
$ catchy run --runtime runc --trace-dir examples/failing-hook/.work/traces examples/failing-hook/.work/bundle
error running prestart hook #0: exit status 42

$ catchy report examples/failing-hook/.work/traces
hook traces: 1

prestart[0] failed
  path: /bin/sh
  exit: 42
  duration: 3ms
  stderr: demo prestart hook: missing required GPU_DEVICE_ID\nhint: set GPU_DEVICE_ID or fix the CDI/device hook config
```

Try the demo with:

```
make demo-failing-hook
```

This project is intentionally focused on OCI runtime hook debugging. It is not a general-purpose container debugger.

## Directory structure

```
catchy/
├── cmd/
│   └── catchy/     # CLI entry points (inspect, wrap, run, report)
├── internal/
│   ├── spec/          # loading and validating OCI config.json
│   ├── hook/          # hook rewriting and wrapper generation
│   └── report/        # reporting and trace summarisation
└── go.mod
```

## Getting started

This repository contains a working bootstrap of the CLI and internal packages.  
The project is written in Go (go 1.20+) and can be compiled as a static binary.

### Build

```
cd catchy
go build ./cmd/catchy
```

### Test

Unit tests run without requiring an OCI runtime:

```
go test ./...
```

Runtime integration tests are opt-in because they require a host that can run OCI containers with `runc` or `crun`:

```
CATCHY_E2E_RUNTIME=1 go test ./test/e2e -v
```

By default the integration test tries `runc` and `crun`. To choose specific runtimes:

```
CATCHY_E2E_RUNTIME=1 CATCHY_E2E_RUNTIMES=runc go test ./test/e2e -v
```

### Commands

* `catchy inspect <path/to/bundle>` – parse `config.json` and output hook definitions with redaction enabled by default.
* `catchy wrap <path/to/bundle>` – rewrite the bundle’s hooks so they point to the wrapper and save the original definitions.
* `catchy restore <path/to/bundle>` – restore `config.json` from `config.json.catchy.bak`.
* `catchy run --runtime <runtime> <path/to/bundle>` – wrap hooks, execute `runtime run -b <bundle> <id>`, and restore the bundle afterward. Prefer repeatable `--runtime-arg ARG` for runtime options; legacy `--runtime-args "..."` is still accepted and uses simple whitespace splitting.
* `catchy report <trace-dir>` – summarise collected hook traces as text, JSON, or YAML.

The wrapper is implemented as a hidden `hook-wrapper` mode in the same binary, so the default `wrap` command can use the current executable as the hook wrapper. Trace files are written as JSON under `<bundle>/.catchy/traces` unless `--trace-dir` is provided. The trace schema is documented in [docs/trace-schema.md](docs/trace-schema.md).

Runtime arguments can be passed without shell quoting ambiguity:

```
catchy run --runtime runc --runtime-arg --root --runtime-arg /tmp/runc-root bundle
```

### Redaction

Trace redaction is enabled by default. `catchy inspect` redacts displayed hook args and env. Before writing trace JSON, `catchy` redacts common sensitive keys in captured hook args, environment variables, OCI state JSON, and simple `key=value` or `key: value` strings in stdout/stderr. Built-in key patterns include `token`, `password`, `secret`, `credential`, `auth`, `authorization`, `api_key`, `access_key`, `private_key`, and `registry_auth`.

Examples:

```
catchy inspect bundle
catchy inspect --no-redact bundle
catchy inspect --redact-key session_id bundle
catchy run --runtime runc bundle
catchy run --no-redact --runtime runc bundle
catchy run --redact-key session_id --runtime runc bundle
```

`--redact-key` can be passed more than once and is also available on `catchy wrap`. Redaction is best-effort hygiene for command output and trace files, not a formal security boundary; review traces before sharing them.

## Contributing

Contributions are welcome!  Please open issues or pull requests.  Before implementing new features, consider reading the OCI runtime specification and related issues to understand the constraints【724359104618359†L86-L98】【790362019473417†L232-L249】.
