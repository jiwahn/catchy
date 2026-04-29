# CDI-like Hook Demo

This example simulates the shape of a CDI/device hook startup failure without implementing CDI and without depending on Docker, containerd, Kubernetes, NVIDIA, or a real device plugin.

The fake hook prints a realistic device resolution error to stderr, reads OCI hook state from stdin, saves that state under the generated `.work/` directory, and exits non-zero.

Run:

```
./examples/cdi-like-hook/run.sh
```

Or through the top-level Makefile:

```
make demo-cdi-like-hook
```

Expected flow:

1. Direct `runc run` fails with a hook error.
2. `catchy run` still fails, but writes hook traces.
3. `catchy diagnose` identifies the failed `prestart` hook, exit code, stderr, redaction status, duration, and likely cause.

Requirements:

* Go
* `runc` in `PATH` or `RUNTIME=/path/to/runc`
* A host/user setup that can run rootless OCI containers

Generated files stay under `examples/cdi-like-hook/.work/` and are ignored by git.
