# Contributing

`catchy` is focused on OCI runtime hook debugging. Keep changes scoped to that problem unless there is clear agreement to broaden the project.

Before sending a change, run:

```
gofmt -w .
go test ./...
go vet ./...
```

Runtime integration tests are opt-in because they require suitable OCI runtime support on the host:

```
CATCHY_E2E_RUNTIME=1 go test ./test/e2e -v
```

Do not commit generated artifacts, including:

* trace files
* `.catchy/`
* `config.json.catchy.bak`
* demo `.work/` directories
* locally built binaries
