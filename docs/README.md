# CrossOS Docs (bead cross-os-qhp.2)

Static Markdown docs served from `docs/` (no framework — the site ships
inside the signed artifacts + repo pages). Non-tech-first copy: no
"CGEventTap", "AXUIElement", "LL hook" in user-facing strings.

## Pages

- `install.md` — install, permissions walkthrough, readiness verify
- `plugins.md` — contribution-driven UI model (§3.6c) for plugin authors
- `safety.md` — kill switch, Reset, trial, uninstall/reversibility
- `capabilities.md` — capability registry (§3.12) overview

The in-app About page (ymh.4) owns version/license/credits — docs LINK to
it, never duplicate. Onboarding reuses the nir.4 permission flow
(Welcome → Enable → Open System Settings → Verify) — no parallel path.
