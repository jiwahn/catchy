#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
RUNTIME=${RUNTIME:-runc}
WORK_DIR="$SCRIPT_DIR/.work"
BUNDLE="$WORK_DIR/bundle"
TRACE_DIR="$WORK_DIR/traces"
CATCHY_BIN="$WORK_DIR/catchy"
ROOTFS="$REPO_ROOT/bundle/rootfs"
DIRECT_ID="catchy-demo-direct-$$"
CATCHY_ID="catchy-demo-catchy-$$"

if ! command -v "$RUNTIME" >/dev/null 2>&1; then
	echo "runtime '$RUNTIME' not found. Install runc or set RUNTIME=/path/to/runtime." >&2
	exit 1
fi

rm -rf "$WORK_DIR"
mkdir -p "$BUNDLE" "$TRACE_DIR"

(cd "$REPO_ROOT" && go build -o "$CATCHY_BIN" ./cmd/catchy)

sed \
	-e "s|__ROOTFS__|$ROOTFS|g" \
	-e "s|\"__UID__\"|$(id -u)|g" \
	-e "s|\"__GID__\"|$(id -g)|g" \
	"$SCRIPT_DIR/bundle/config.json" > "$BUNDLE/config.json"

cleanup() {
	"$RUNTIME" delete -f "$DIRECT_ID" >/dev/null 2>&1 || true
	"$RUNTIME" delete -f "$CATCHY_ID" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "== Direct runtime run =="
echo "+ $RUNTIME run -b $BUNDLE $DIRECT_ID"
set +e
"$RUNTIME" run -b "$BUNDLE" "$DIRECT_ID" >"$WORK_DIR/direct.stdout" 2>"$WORK_DIR/direct.stderr"
direct_status=$?
set -e
cat "$WORK_DIR/direct.stdout"
cat "$WORK_DIR/direct.stderr" >&2
echo "direct exit status: $direct_status"

echo
echo "== catchy run =="
echo "+ $CATCHY_BIN run --runtime $RUNTIME --wrapper $CATCHY_BIN --trace-dir $TRACE_DIR --id $CATCHY_ID $BUNDLE"
set +e
"$CATCHY_BIN" run \
	--runtime "$RUNTIME" \
	--wrapper "$CATCHY_BIN" \
	--trace-dir "$TRACE_DIR" \
	--id "$CATCHY_ID" \
	"$BUNDLE" >"$WORK_DIR/catchy.stdout" 2>"$WORK_DIR/catchy.stderr"
catchy_status=$?
set -e
cat "$WORK_DIR/catchy.stdout"
cat "$WORK_DIR/catchy.stderr" >&2
echo "catchy run exit status: $catchy_status"

echo
echo "== catchy report =="
"$CATCHY_BIN" report "$TRACE_DIR"
if ! find "$TRACE_DIR" -name '*.json' -type f | grep -q .; then
	echo
	echo "No trace files were produced. That usually means the runtime failed before invoking hooks"
	echo "(for example, this user/host cannot create the namespaces needed to start an OCI container)."
fi

echo
echo "Generated demo files are under $WORK_DIR and are ignored by git."
