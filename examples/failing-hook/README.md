# failing-hook demo

This example demonstrates the narrow problem `catchy` is meant to solve: an OCI `prestart` hook fails, and the runtime error alone does not give enough context to debug it quickly.

The committed `bundle/config.json` contains a `prestart` hook that intentionally fails:

```
/bin/sh -c "echo 'demo prestart hook: missing required GPU_DEVICE_ID' >&2; ...; exit 42"
```

The script creates a throwaway bundle under `.work/`, points it at the repository's small `bundle/rootfs`, runs it directly with `runc`, then runs it through `catchy`.

Run:

```
make demo-failing-hook
```

Or:

```
RUNTIME=runc examples/failing-hook/run.sh
```

Requirements:

* Go
* `runc` in `PATH` or `RUNTIME=/path/to/runc`
* A host/user setup that can run rootless OCI containers

Generated files stay under `examples/failing-hook/.work/` and are ignored by git.
