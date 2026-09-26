// The Wails runtime, stubbed for the browser.
//
// Two members of it are called on paths a browser can actually reach, and both
// fail hard there, which is how the switcher overlay came up blank in a
// preview: `Window.Show()` is awaited on every trigger, its rejection is caught
// and turned into a fault, and a fault replaces the panel — so the one screen
// the overlay exists to show was the one screen the preview could not show.
//
// Everything else is absent on purpose. A stub that answers a call the product
// does not make would let the preview look right where the product is not, and
// the point of this harness is to see what a person would see.

export const Window = {
  Show: async (): Promise<void> => {},
  Hide: async (): Promise<void> => {},
  Center: async (): Promise<void> => {},
  SetPosition: async (): Promise<void> => {},
  SetSize: async (): Promise<void> => {},
  GetPosition: async (): Promise<{ x: number; y: number }> => ({ x: 0, y: 0 }),
  GetSize: async (): Promise<{ width: number; height: number }> => ({ width: 0, height: 0 }),
}

export const Clipboard = {
  SetText: async (): Promise<void> => {},
  GetText: async (): Promise<string> => '',
}

export const Application = {
  Quit: async (): Promise<void> => {},
}
