#!/usr/bin/env sh
set -eu

STATE_OUT=${CATCHY_FAKE_CDI_STATE_OUT:-}

if [ -n "$STATE_OUT" ]; then
	cat > "$STATE_OUT"
else
	cat >/dev/null
fi

echo 'fake CDI hook: simulating device injection failure' >&2
echo 'CDI hook failed: device "vendor.com/gpu=0" not found' >&2
echo 'hint: check CDI device spec path or device plugin configuration' >&2

exit 42
