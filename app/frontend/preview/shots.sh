#!/bin/bash
# Screenshot the preview shell with headless Chrome. One page of the nav per
# invocation, so each shot is the window as a person would meet it.
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
OUT="/tmp/crossos-shots"
BASE="http://127.0.0.1:5299/preview/index.html"
mkdir -p "$OUT"

# shoot <name> <query>
"$CHROME" --headless --disable-gpu --hide-scrollbars \
  --window-size=1100,720 --force-device-scale-factor=2 \
  --virtual-time-budget=4000 \
  --screenshot="$OUT/$1.png" "$BASE?$2" >/dev/null 2>&1
echo "$OUT/$1.png"
