# catchy

`catchy` is a small CLI for debugging container startup failures caused by OCI runtime hooks.

It works at the OCI bundle level: inspect hooks, preflight-check hook definitions, wrap hooks with a tracing wrapper, run the bundle through an OCI runtime, then report or diagnose the captured traces.

It is focused on runtime hooks. It is not a general container debugger.

## Install

```sh
go install github.com/jiwahn/catchy/cmd/catchy@latest
```

Local build:

```sh
go build -o catchy ./cmd/catchy
```

## Quick Start

```sh
catchy inspect bundle
catchy check bundle
catchy run --runtime runc bundle
catchy diagnose bundle/.catchy/traces
catchy report bundle/.catchy/traces
```

## Commands

* `catchy inspect <bundle>`: show OCI hooks with redaction enabled by default.
* `catchy check <bundle>`: validate hook paths, permissions, interpreters, and timeouts.
* `catchy wrap <bundle>`: rewrite hooks to run through the tracing wrapper.
* `catchy restore <bundle>`: restore `config.json` from `config.json.catchy.bak`.
* `catchy run --runtime <runtime> <bundle>`: wrap, run, trace, and restore.
* `catchy diagnose <trace-dir>`: show the failed hook, output, exit data, and hints.
* `catchy report <trace-dir>`: print trace reports as text, JSON, or YAML.

## Demos

```sh
make demo-failing-hook
make demo-cdi-like-hook
```

* `examples/failing-hook`: minimal synthetic failing hook.
* `examples/cdi-like-hook`: simulated CDI/device hook-style failure, not real CDI integration.

## Docs

* [Usage and motivation](docs/usage.md)
* [Trace schema](docs/trace-schema.md)
* [Contributing](CONTRIBUTING.md)

## Test

```sh
go test ./...
```

Runtime e2e tests are opt-in:

```sh
CATCHY_E2E_RUNTIME=1 go test ./test/e2e -v
```
