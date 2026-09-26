#!/bin/bash
# Screenshot the preview shell with headless Chrome, at the app's real window
# size. One page of the nav per invocation, so each shot is the window as a
# person would meet it.
#
#   ./preview/shots.sh home "state=ready&page=Home"
#   ./preview/shots.sh home "state=ready&page=Home" --dark
#   ./preview/shots.sh home "state=ready&page=Home" --contrast --dark
#
# --dark and --contrast are CDP media EMULATION, not a rewrite of the
# stylesheet: the page runs under a real `prefers-color-scheme: dark` rather
# than with its media query edited out from under it. That distinction matters
# for this app in particular, because the palette's correctness IS a question
# about which block wins the cascade.
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
OUT="/tmp/crossos-shots"
BASE="http://127.0.0.1:5299/preview/index.html"
mkdir -p "$OUT"

NAME="$1"; QUERY="$2"; shift 2 2>/dev/null
DARK=0; CONTRAST=0
for flag in "$@"; do
  case "$flag" in
    --dark) DARK=1 ;;
    --contrast) CONTRAST=1 ;;
  esac
done

# The plain path stays plain: no CDP, no daemon, just Chrome's own screenshot.
if [ "$DARK" -eq 0 ] && [ "$CONTRAST" -eq 0 ]; then
  "$CHROME" --headless --disable-gpu --hide-scrollbars \
    --window-size=1100,720 --force-device-scale-factor=2 \
    --virtual-time-budget=4000 \
    --screenshot="$OUT/$NAME.png" "$BASE?$QUERY" >/dev/null 2>&1
  echo "$OUT/$NAME.png"
  exit 0
fi

"$CHROME" --headless --disable-gpu --hide-scrollbars \
  --window-size=1100,720 --force-device-scale-factor=2 \
  --remote-debugging-port=9344 --user-data-dir=/tmp/crossos-shots-cdp \
  "$BASE?$QUERY" >/dev/null 2>&1 &
CHROME_PID=$!
trap 'kill $CHROME_PID 2>/dev/null' EXIT

for _ in $(seq 40); do
  curl -s http://127.0.0.1:9344/json/list >/dev/null 2>&1 && break
  sleep 0.25
done

node -e '
const [out, dark, contrast] = process.argv.slice(1)
;(async () => {
  const targets = await (await fetch("http://127.0.0.1:9344/json/list")).json()
  const page = targets.find((t) => t.type === "page")
  const ws = new WebSocket(page.webSocketDebuggerUrl)
  let id = 0
  const pending = new Map()
  const send = (method, params) =>
    new Promise((res) => { const n = ++id; pending.set(n, res); ws.send(JSON.stringify({ id: n, method, params })) })
  ws.onmessage = (e) => {
    const m = JSON.parse(e.data)
    if (m.id && pending.has(m.id)) { pending.get(m.id)(m.result); pending.delete(m.id) }
  }
  await new Promise((r) => (ws.onopen = r))
  // The real API for both. prefers-color-scheme and prefers-contrast are
  // media features, and this is how an engine is asked to report them.
  const features = []
  if (dark === "1") features.push({ name: "prefers-color-scheme", value: "dark" })
  if (contrast === "1") features.push({ name: "prefers-contrast", value: "more" })
  if (features.length) await send("Emulation.setEmulatedMedia", { features })
  await send("Page.reload", {})
  await new Promise((r) => setTimeout(r, 2500))
  const shot = await send("Page.captureScreenshot", { format: "png" })
  require("fs").writeFileSync(out, Buffer.from(shot.data, "base64"))
  ws.close()
})()
' "$OUT/$NAME.png" "$DARK" "$CONTRAST"
echo "$OUT/$NAME.png"
