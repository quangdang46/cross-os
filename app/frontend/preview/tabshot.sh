#!/bin/bash
# Tab N times with a real key event and screenshot, so :focus-visible is
# actually engaged. A programmatic .focus() does NOT match :focus-visible on a
# <button>, so a shot taken that way shows no ring and proves nothing.
set -e
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
URL="$1"; NAME="$2"; TABS="${3:-3}"
OUT=/tmp/crossos-shots

"$CHROME" --headless --disable-gpu --hide-scrollbars --window-size=1100,720 \
  --force-device-scale-factor=2 --remote-debugging-port=9333 \
  --user-data-dir=/tmp/crossos-cdp "$URL" >/dev/null 2>&1 &
CHROME_PID=$!
trap 'kill $CHROME_PID 2>/dev/null' EXIT

for _ in $(seq 40); do
  curl -s http://127.0.0.1:9333/json/list >/dev/null 2>&1 && break
  sleep 0.25
done

node -e '
const tabs = +process.argv[1]
;(async () => {
  const targets = await (await fetch("http://127.0.0.1:9333/json/list")).json()
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
  await new Promise((r) => setTimeout(r, 1500))
  for (let i = 0; i < tabs; i++) {
    for (const type of ["rawKeyDown", "char", "keyUp"]) {
      await send("Input.dispatchKeyEvent", {
        type, key: "Tab", code: "Tab", windowsVirtualKeyCode: 9, nativeVirtualKeyCode: 9, text: "\t",
      })
    }
    await new Promise((r) => setTimeout(r, 120))
  }
  const { result } = await send("Runtime.evaluate", {
    expression: "(()=>{const a=document.activeElement;const s=getComputedStyle(a);return a.className+\" | outline=\"+s.outlineWidth+\" \"+s.outlineStyle+\" \"+s.outlineColor+\" offset=\"+s.outlineOffset})()",
    returnByValue: true,
  })
  console.log(result.value)
  const shot = await send("Page.captureScreenshot", { format: "png" })
  require("fs").writeFileSync(process.argv[2], Buffer.from(shot.data, "base64"))
  ws.close()
})()
' "$TABS" "$OUT/$NAME.png"
echo "$OUT/$NAME.png"
