#!/bin/sh
set -e

# When DISPLAY is set, wait for the X server (Xvfb sidecar) to be ready.
# Xvfb and Chrome start concurrently in the pod, so a brief race is expected.
if [ -n "$DISPLAY" ]; then
    i=0
    while [ $i -lt 40 ]; do
        xdpyinfo >/dev/null 2>&1 && break
        sleep 0.25
        i=$((i + 1))
    done
fi

# Filter out our managed CDP port/address flags from the args so we can
# set our own values. Remaining args (--headless, --window-size, etc.) pass through.
chrome_args=""
for arg in "$@"; do
    case "$arg" in
        --remote-debugging-address=* | --remote-debugging-port=*) ;;
        *) chrome_args="$chrome_args $arg" ;;
    esac
done

# Start socat in the background to forward the external CDP port (9222) to
# Chrome's internal port (9223). This works around Chromium binding DevTools
# to 127.0.0.1 even when --remote-debugging-address=0.0.0.0 is set.
socat TCP4-LISTEN:9222,fork TCP4:127.0.0.1:9223 &

# Start Chrome in the foreground. The container lives as long as Chrome does.
# shellcheck disable=SC2086
exec chromium \
    --remote-debugging-address=0.0.0.0 \
    --remote-debugging-port=9223 \
    $chrome_args
