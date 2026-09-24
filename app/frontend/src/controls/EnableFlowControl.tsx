// The kind earlier builds of the daemon served for the first-run flow.
//
// The Activity page (ActivityPage, pages.go) and the Welcome page
// (OnboardingFlow, onboarding.go) both declare `enableFlow`. The flow they
// declared is now drawn by the wizard, which reads the daemon's step machine
// instead of listing the steps as text — so this file is a NAME, not a second
// renderer: one implementation, two spellings of one kind.
//
// The alias exists so a daemon that has not been rebuilt alongside the shell
// still draws a working wizard rather than landing in UnsupportedControl, where
// the page would be self-describing but empty. Deleting the spelling is a Go
// change (the two `kind` fields above) and not a reason to break a page in the
// meantime.

export { WizardControl as EnableFlowControl } from './WizardControl'
