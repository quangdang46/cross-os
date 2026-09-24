// The typed facade over the Wails-bound Service (bead cross-os-itq).
//
// This module is the only place in the frontend that names the generated
// bindings. Everything else — every control, the context they receive — talks
// to ServiceApi from ../types/controls, so the shapes the controls code
// against live in one reviewable file rather than in generated JavaScript.
//
// The generated Service is re-exported unchanged so App.tsx can import it from
// here instead of reaching into ../../bindings by path: when a page, a control
// and the app all reach the daemon through one module, the day the binding
// names change is the day one file changes.
//
// The check below only bites because tsconfig sets allowJs. Without it the
// generator's .js modules resolve to `any`, this assignment stops being a
// check, and a renamed model field reaches the window as `undefined` — which is
// exactly how the PascalCase row types in types/controls.ts survived a green
// `tsc --noEmit` while naming properties the generator never emits.

import { Service } from '../../bindings/crossos/app/backend'
import type { ServiceApi } from '../types/controls'

export { Service }

/**
 * The bound service, typed as the control-facing surface. The annotation is a
 * real check, not a cast: if a Go method changes its arity or its return, the
 * Wails-generated signature stops satisfying ServiceApi here and the build
 * fails — which is the moment a renamed field should be noticed, rather than
 * discovered as an `undefined` in a settings row.
 */
export const service: ServiceApi = Service
