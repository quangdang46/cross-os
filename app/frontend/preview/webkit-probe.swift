// CrossOS — does WebKit resolve the palette the stylesheet hands it?
//
// The whole colour fix rests on one claim: in a WKWebView, `CanvasText`,
// `ButtonFace` and `AccentColor` resolve to real colours, so the @supports
// block hands appearance and accent back to macOS. Every measurement so far has
// been taken in Chrome, which REPORTS supporting `AccentColor` and resolves
// none of its keywords — so the whole palette there collapses to the initial
// value. That is a property of Chrome, not of the app.
//
// This loads the preview in a real WKWebView — the same engine the shipped
// window uses — and reads the resolved values back out. No screenshot, no
// pixels: the numbers are the answer.
//
//   swiftc -O preview/webkit-probe.swift -o /tmp/webkit-probe && /tmp/webkit-probe <url>

import AppKit
import WebKit

let url = URL(string: CommandLine.arguments.count > 1
    ? CommandLine.arguments[1]
    : "http://127.0.0.1:5299/preview/index.html?state=ready&page=Home")!

final class Probe: NSObject, WKNavigationDelegate {
    let view: WKWebView
    var done = false

    override init() {
        // The appearance has to be set on the APPLICATION, before the web view
        // exists. Setting it on the view alone leaves the system colour keywords
        // resolving against whatever the process started with, which reads as a
        // dark app drawing a light palette — a probe artifact, not a defect.
        let dark = CommandLine.arguments.contains("--dark")
        NSApplication.shared.appearance = NSAppearance(named: dark ? .darkAqua : .aqua)
        let config = WKWebViewConfiguration()
        view = WKWebView(frame: NSRect(x: 0, y: 0, width: 1100, height: 720), configuration: config)
        super.init()
        view.navigationDelegate = self
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        // One turn for the shell's first poll to land and paint.
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.5) { self.read() }
    }

    func read() {
        if CommandLine.arguments.contains("--contrast") { return contrast() }
        // A control page publishes its own answer in <pre id="out">, and that
        // answer is about THIS probe's validity — whether the system colours
        // follow the appearance at all. So it wins over the token list.
        view.evaluateJavaScript("document.getElementById('out')?.textContent ?? ''") { value, error in
            if let text = value as? String, !text.isEmpty {
                print(text)
                exit(0)
            }
            if let error { print("PROBE ERROR: \(error)"); exit(1) }
            self.readTokens()
        }
    }

    func readTokens() {
        // A probe element is the only way to get a RESOLVED colour out of a
        // custom property: getComputedStyle on :root returns the token's own
        // text, which for `AccentColor` is the keyword and not the colour. Set
        // each token on one element and read the computed background back.
        let js = """
        (() => {
          const cs = getComputedStyle(document.documentElement);
          const tokens = ['--bg-window','--bg-sidebar','--bg-raised','--bg-sunken',
                          '--fg-primary','--fg-secondary','--fg-tertiary','--fg-onaccent',
                          '--accent','--sel-bg','--line-strong','--line-default','--line-hairline',
                          '--ok','--warn','--danger','--live'];
          const resolved = {};
          const probe = document.createElement('span');
          probe.style.display = 'none';
          document.body.appendChild(probe);
          for (const t of tokens) {
            probe.style.backgroundColor = '';
            probe.style.color = '';
            probe.style.backgroundColor = `var(${t})`;
            probe.style.color = `var(${t})`;
            // backgroundColor keeps the colour if it is a colour, and 'rgba(0, 0, 0, 0)'
            // if the token is not one. Try colour, then fall back to background.
            let v = getComputedStyle(probe).backgroundColor;
            if (v === 'rgba(0, 0, 0, 0)') v = getComputedStyle(probe).color;
            resolved[t] = v;
          }
          probe.remove();
          const pick = (sel, prop) => {
            const el = document.querySelector(sel);
            return el ? getComputedStyle(el)[prop] : '(none)';
          };
          return JSON.stringify({
            appearance: window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light',
            resolved,
            painted: {
              cardBg: pick('.card', 'backgroundColor'),
              railBg: pick('.sections', 'backgroundColor'),
              listBg: pick('.ctl-list', 'backgroundColor'),
              pageInk: pick('.page-title', 'color'),
              capInk: pick('.nav-group-title', 'color'),
            },
          }, null, 2);
        })()
        """
        view.evaluateJavaScript(js) { value, error in
            if let error { print("PROBE ERROR: \(error)"); exit(1) }
            print(value as? String ?? "(no value)")
            exit(0)
        }
    }

    /// The measurement that actually settles 1.4.11: a control's REAL painted
    /// border against the REAL surface it sits on, both read off live elements
    /// rather than off a token. A token read in isolation is not the question —
    /// `ButtonBorder` is a dynamic macOS colour and resolves over whatever is
    /// behind it, so only the pair means anything.
    func contrast() {
        let js = """
        (() => {
          const px = (c) => {
            const m = c.match(/[\\d.]+/g);
            if (!m) return null;
            return { r: +m[0], g: +m[1], b: +m[2], a: m[3] === undefined ? 1 : +m[3] };
          };
          const over = (fg, bg) => ({
            r: fg.r * fg.a + bg.r * (1 - fg.a),
            g: fg.g * fg.a + bg.g * (1 - fg.a),
            b: fg.b * fg.a + bg.b * (1 - fg.a),
            a: 1,
          });
          const lum = (c) => {
            const f = (v) => { v /= 255; return v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
            return 0.2126 * f(c.r) + 0.7152 * f(c.g) + 0.0722 * f(c.b);
          };
          const ratio = (a, b) => {
            const la = lum(a);
            const lb = lum(b);
            const hi = Math.max(la, lb);
            const lo = Math.min(la, lb);
            return +((hi + 0.05) / (lo + 0.05)).toFixed(2);
          };
          // The surface an element is actually painted on, walking up for the
          // first ancestor that has one.
          const behind = (el) => {
            let n = el;
            while (n && n !== document.documentElement) {
              const c = px(getComputedStyle(n).backgroundColor);
              if (c && c.a > 0) return c;
              n = n.parentElement;
            }
            return { r: 255, g: 255, b: 255, a: 1 };
          };
          const out = {};
          const measure = (label, sel, prop) => {
            const el = document.querySelector(sel);
            if (!el) { out[label] = '(no such element)'; return; }
            const s = getComputedStyle(el);
            const raw = px(s[prop]);
            const bg = behind(el.parentElement ?? el);
            if (!raw) { out[label] = { border: s[prop], ratio: null }; return; }
            out[label] = {
              raw: s[prop],
              composited: `rgb(${Math.round(over(raw, bg).r)}, ${Math.round(over(raw, bg).g)}, ${Math.round(over(raw, bg).b)})`,
              surfaceBehind: `rgb(${Math.round(bg.r)}, ${Math.round(bg.g)}, ${Math.round(bg.b)})`,
              ratio: ratio(over(raw, bg), bg),
            };
          };
          measure('input.border', '.ctl-input', 'borderTopColor');
          measure('select.border', '.ctl-select', 'borderTopColor');
          measure('toggle.border', '.ctl-toggle', 'borderTopColor');
          measure('button.border', '.ctl-button, .ctl-actions button', 'borderTopColor');
          return JSON.stringify(out, null, 2);
        })()
        """
        view.evaluateJavaScript(js) { value, error in
            if let error { print("CONTRAST ERROR: \(error)"); exit(1) }
            print(value as? String ?? "(no value)")
            exit(0)
        }
    }
}

let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let probe = Probe()
probe.view.load(URLRequest(url: url))
DispatchQueue.main.asyncAfter(deadline: .now() + 25) {
    print("TIMEOUT — the page never finished loading")
    exit(2)
}
app.run()
