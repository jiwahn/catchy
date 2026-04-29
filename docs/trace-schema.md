# Trace Schema

`catchy` writes one JSON trace file for each OCI hook execution observed through `catchy hook-wrapper`.

This document describes trace schema version `2`.

## Compatibility

Trace consumers should use `traceVersion` to branch on known schema semantics and should tolerate unknown fields. `traceVersion` may change when fields or their meanings change.

## Fields

| Field | Type | Description |
| --- | --- | --- |
| `timestamp` | string | UTC timestamp when the wrapper began recording the hook execution. |
| `hookStage` | string | OCI hook stage, such as `prestart`, `createRuntime`, `startContainer`, `poststart`, or `poststop`. |
| `hookIndex` | number | Hook index within the stage array from `config.json`. |
| `path` | string | Original hook executable path. |
| `args` | array of strings | Original hook argv, after redaction when enabled. |
| `env` | array of strings | Original hook environment entries, after redaction when enabled. |
| `timeout` | number | Original hook timeout in seconds, when configured. |
| `durationMs` | number | Hook execution duration in milliseconds. |
| `exitCode` | number | Hook process exit code. `0` means success. |
| `signal` | string | Signal name when the hook terminated due to a signal. |
| `error` | string | Wrapper-side execution error, after redaction when enabled. |
| `timedOut` | boolean | `true` when the wrapper killed the hook after its timeout. |
| `stdout` | string | Captured hook stdout, after redaction when enabled. |
| `stderr` | string | Captured hook stderr, after redaction when enabled. |
| `state` | object | OCI hook state JSON read from stdin, after recursive redaction when enabled. |
| `redacted` | boolean | `true` when redaction was applied before writing the trace. |
| `redactionKeys` | array of strings | Active sensitive key patterns used for best-effort redaction. |
| `traceVersion` | number | Trace schema version. Current version is `2`. |

## Redaction

Redaction is enabled by default. When `redacted` is `true`, `catchy` redacted data before writing the trace file.

Redaction applies to hook args, hook env, OCI state JSON, stdout, stderr, and wrapper error text. For JSON objects, values are replaced with `"<redacted>"` when their key matches a sensitive pattern. Nested objects and arrays are processed recursively.

`redactionKeys` lists the active key patterns, including built-in defaults and any extra keys supplied with `--redact-key`.

This is best-effort trace hygiene, not a formal security boundary. Review trace files before sharing them.

## Successful Hook Example

```json
{
  "timestamp": "2026-04-30T00:00:00Z",
  "hookStage": "prestart",
  "hookIndex": 0,
  "path": "/usr/local/bin/setup-device",
  "args": [
    "setup-device",
    "--mode=check"
  ],
  "env": [
    "NORMAL=ok",
    "TOKEN=<redacted>"
  ],
  "timeout": 5,
  "durationMs": 12,
  "exitCode": 0,
  "stdout": "device check ok\n",
  "state": {
    "id": "demo-container",
    "annotations": {
      "registry_auth": "<redacted>"
    }
  },
  "redacted": true,
  "redactionKeys": [
    "token",
    "password",
    "secret",
    "registry_auth"
  ],
  "traceVersion": 2
}
```

## Failed Hook Example

```json
{
  "timestamp": "2026-04-30T00:00:05Z",
  "hookStage": "prestart",
  "hookIndex": 0,
  "path": "/bin/sh",
  "args": [
    "sh",
    "-c",
    "echo password=<redacted> >&2; exit 42"
  ],
  "env": [
    "PASSWORD=<redacted>"
  ],
  "timeout": 5,
  "durationMs": 3,
  "exitCode": 42,
  "error": "exit status 42",
  "stdout": "",
  "stderr": "password=<redacted>\n",
  "state": {
    "pid": 12345
  },
  "redacted": true,
  "redactionKeys": [
    "token",
    "password",
    "secret"
  ],
  "traceVersion": 2
}
```
