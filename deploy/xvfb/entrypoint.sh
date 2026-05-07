#!/bin/sh
set -e

exec Xvfb ":${DISPLAY_NUM}" \
    -screen 0 "${SCREEN_WIDTH}x${SCREEN_HEIGHT}x${SCREEN_DEPTH}" \
    -ac \
    -nolisten tcp \
    -nolisten unix \
    +extension RANDR
