# Attribution — pcfy-my-mac setup-sequencing port (bead w6-frontend-setup-renderers)

Source repository: raxigan/pcfy-my-mac
Source commit: 7ebef7f8df40a78b3c5761d10bceb1abaf96c1ce
Source file: cmd/task/task.go
Original license: MIT
Original copyright: Copyright (c) 2024 Raxigan
CrossOS destination: app/frontend/src/controls/WizardControl.tsx (the ordered step list, one open at a time, and the finish write)
Modification: structural port, no code copied. What transfers is the SHAPE — a list of named steps, each with its own body, run in order, with a finish at the end (`Task{Name, Execute}` at task.go:18-22, and the ordered question list at survey.go:8-14). The wizard's steps, their names and their actions are the PAGE's declaration (the `steps` and `actions` fields a Go UIContribution carries), never a copy of the reference's four questions, and the step verdicts are the daemon's, not the shell's.
Reason for modification: the reference is a survey-driven installer for one target machine layout; CrossOS renders a declared schema in a webview, and §9.11 records pcfy-my-mac as SCRIPTS ONLY — installer sequencing reference, nothing of it ships in the Core binary. What is ported here is layout and sequencing order, which is the reference's UX and no code.
CrossOS license: MIT

Source repository: raxigan/pcfy-my-mac
Source commit: 7ebef7f8df40a78b3c5761d10bceb1abaf96c1ce
Source file: cmd/param/survey.go
Original license: MIT
Original copyright: Copyright (c) 2024 Raxigan
CrossOS destination: app/frontend/src/controls/WizardControl.tsx (one question per decision, in order, each with its own help text)
Modification: structural port, no code copied — the wizard's step list is the declared `steps` array rendered in order, and the body of the open step carries the page's own note and the writes the page declared. The reference's `survey.Select` / `survey.MultiSelect` prompts, its "Recommended" descriptions and its OS-default options are not ported: CrossOS asks nothing, it renders what the daemon sent.
Reason for modification: language and medium port (Go survey prompts in a CLI become declared controls in a settings window); the reference's questions are about one reference machine, and CrossOS's questions are the daemon's page schema.
CrossOS license: MIT

No pcfy-my-mac source is included; the LICENSE is recorded here for the
sequencing reference only.
