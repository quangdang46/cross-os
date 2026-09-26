import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'

// A second vite config used ONLY to look at the shell in a browser. It swaps
// src/lib/service for the test fixture's stub and serves public/ as the static
// root, so /style.css resolves the same way the packaged document does.
const root = fileURLToPath(new URL('..', import.meta.url))
const stub = fileURLToPath(new URL('./service-stub.ts', import.meta.url))
const wailsStub = fileURLToPath(new URL('./wails-stub.ts', import.meta.url))

// An alias key is matched against the import SPECIFIER, and the specifier the
// shell writes is './lib/service' — a path, not a prefix anyone else uses. A
// plugin hook is the honest way to intercept exactly that one seam and nothing
// else, so a test fixture cannot quietly become the app's real backend.
//
// Two seams, not one. The second is @wailsio/runtime, whose Window.Show() is
// awaited on every switcher trigger; in a browser it rejects, the overlay turns
// that into a fault, and a fault replaces the panel — so the preview could not
// show the one screen the overlay exists for.
const swapSeams = (): Plugin => ({
  name: 'preview-seams',
  enforce: 'pre',
  resolveId(source) {
    if (/(^|\/)lib\/service$/.test(source)) return stub
    if (source === '@wailsio/runtime' || /@wailsio\/runtime$/.test(source)) return wailsStub
    return null
  },
})

export default defineConfig({
  root,
  publicDir: 'public',
  plugins: [swapSeams(), react()],
  server: { host: '127.0.0.1', port: 5299, strictPort: true },
})
