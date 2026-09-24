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
  },
});
