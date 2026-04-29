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

This repository currently contains the skeleton for the CLI and internal packages.  
The project is intended to be written in Go (go 1.20+) and compiled as a static binary.  
Since this is an early bootstrap, the code is not yet functional – it lays out the foundation for further development.

### Build

```
cd catchy
go build ./cmd/catchy
```

### Commands (planned)

* `catchy inspect <path/to/bundle>` – parse `config.json` and output hook definitions.
* `catchy wrap <path/to/bundle>` – rewrite the bundle’s hooks so they point to the wrapper and save the original definitions.
* `catchy run --runtime <runtime> <path/to/bundle>` – wrap hooks and execute `create`/`start` via the chosen runtime.
* `catchy report <trace-dir>` – summarise collected hook traces.

## Contributing

Contributions are welcome!  Please open issues or pull requests.  Before implementing new features, consider reading the OCI runtime specification and related issues to understand the constraints【724359104618359†L86-L98】【790362019473417†L232-L249】.