import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
//
// The test block is declared here rather than in a second config file so there
// is one place that knows which plugins this frontend runs. The Wails plugin
// binds the generated ../bindings directory to the dev and build pipelines and
// has nothing to say about a jsdom render, so it is left out under vitest:
// that is also what lets the suite run on a checkout where the bindings have
// not been generated yet, because the shell's only binding import is mocked.
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), ...(process.env.VITEST ? [] : [wails("./bindings")])],
  // Two documents, so two entries. The settings shell is index.html; the
  // switcher overlay is src/switcher.html, which is a WINDOW of its own rather
  // than a route inside the settings window — a switcher summoned while the
  // settings window is already up has to be a second window, and a second
  // window is a second document. Naming the settings entry explicitly keeps the
  // default from silently becoming whichever html file sorted first.
  build: {
    rollupOptions: {
      input: {
        main: "index.html",
        switcher: "src/switcher.html",
      },
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    restoreMocks: true,
    // Vitest's default is 5s, which is a budget for a test that checks one
    // thing. src/e2e/firstRun.test.tsx is the exception: it drives the whole
    // first run — land on the wizard, pick a profile, open System Settings,
    // come back, verify, read back a decision, resolve a conflict — across
    // eleven pages with a real settle wait in the middle, and it failed
    // intermittently on a machine with four node processes running.
    //
    // The budget is HERE rather than passed per test because the per-test
    // argument had no effect under vitest 5.0.1, which is why this comment
    // exists at all. And it is not set high enough to hide a hang: a test that
    // never settles still fails, it just takes twenty seconds to say so.
    testTimeout: 20000,
  },
});
