# catchy

`catchy` is a small CLI for debugging container startup failures caused by OCI runtime hooks.

It works at the OCI bundle level: inspect hooks, preflight-check hook definitions, wrap hooks with a tracing wrapper, run the bundle through an OCI runtime, then report or diagnose the captured traces.

It is focused on runtime hooks. It is not a general container debugger.

It also has filesystem-based helpers for locating containerd runtime v2 task bundles.

## Features

* Inspect and preflight-check OCI hook definitions.
* Wrap hooks to capture execution traces.
* Report and diagnose hook failures with redaction enabled by default.
* Locate containerd runtime v2 task bundles from namespace and container ID.
* Trace OCI image annotations and labels to spot metadata propagation gaps.

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
* `catchy bundle-path --namespace <ns> --id <id>`: print a containerd runtime v2 bundle path.
* `catchy check-containerd --namespace <ns> --id <id>`: check a containerd task bundle.
* `catchy inspect-containerd --namespace <ns> --id <id>`: inspect a containerd task bundle.
* `catchy diagnose-containerd --namespace <ns> --id <id>`: diagnose traces for a containerd task bundle.
* `catchy trace-metadata <image>`: show image manifest annotations, config labels, and propagation notes.

## containerd helpers

These helpers look under `/run/containerd/io.containerd.runtime.v2.task/<namespace>/<id>`. They do not query the containerd API, so the bundle must still exist on disk. Kubernetes usually uses namespace `k8s.io`.

```sh
sudo catchy bundle-path --namespace default --id test
sudo catchy check-containerd --namespace default --id test
sudo catchy inspect-containerd --namespace default --id test
```

## Metadata tracing

OCI images can store metadata in manifest annotations and config labels. Container runtimes often do not propagate manifest annotations; containerd typically uses config labels, not manifest annotations.

```sh
catchy trace-metadata harbor.example.com/test:latest
```

The command is read-only. It uses `crane`, then `skopeo`, then `docker`. Docker fallback usually shows local image config labels only and usually cannot show remote manifest annotations.

Example output includes the source tool and manifest media type:

```text
image: harbor.example.com/test:latest
source: crane
media type: application/vnd.oci.image.index.v1+json
```

If the image resolves to an index or manifest list, top-level annotations may be index-level metadata; platform-specific manifest annotations may need platform selection in a future version.

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
