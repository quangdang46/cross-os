# Attribution — pcfy-my-mac setup-sequencing port (bead w6-frontend-setup-renderers)

Source repository: raxigan/pcfy-my-mac
Source commit: 7ebef7f8df40a78b3c5761d10bceb1abaf96c1ce
Source file: cmd/task/task.go
Original license: MIT
Original copyright: Copyright (c) 2024 Raxigan
CrossOS destination: app/frontend/src/controls/WizardControl.tsx (the ordered step list, one open at a time, and the finish write)
Modification: structural port, no code copied. What transfers is the SHAPE — a list of named steps, each with its own body, run in order, with a finish at the end (the step as a name plus a body, `Task{Name, Execute}` at task.go:17-20; the list those steps make and the loop that runs them in order at launcher.go:32-66; and the ordered question list at survey.go:7-63). The wizard's steps, their names and their actions are the PAGE's declaration (the `steps` and `actions` fields a Go UIContribution carries), never a copy of the reference's four questions, and the step verdicts are the daemon's, not the shell's.
Reason for modification: the reference is a survey-driven installer for one target machine layout; CrossOS renders a declared schema in a webview, and §9.11 records pcfy-my-mac as SCRIPTS ONLY — installer sequencing reference, nothing of it ships in the Core binary. What is ported here is layout and sequencing order, which is the reference's UX and no code.
CrossOS license: MIT

Source repository: raxigan/pcfy-my-mac
Source commit: 7ebef7f8df40a78b3c5761d10bceb1abaf96c1ce
Source file: cmd/param/survey.go
Original license: MIT
Original copyright: Copyright (c) 2024 Raxigan
CrossOS destination: app/frontend/src/controls/WizardControl.tsx (one question per decision, in order, each with its own help text)
Modification: structural port, no code copied — what transfers is the per-question body: the reference's ordered `questions` slice (survey.go:7-63) is a list in which every element carries its own Name, its own Prompt and its own Help string, so a question is never explained by its neighbour and the explanation sits with the decision it belongs to. The wizard's step list is the same shape: the declared `steps` array rendered in order, the open step's own body carrying the page's own note and the writes the page declared. The reference's `survey.Select` / `survey.MultiSelect` prompts, its "Recommended" descriptions and its OS-default options are not ported: CrossOS asks nothing, it renders what the daemon sent.
Reason for modification: language and medium port (Go survey prompts in a CLI become declared controls in a settings window), and §9.11 records pcfy-my-mac as SCRIPTS ONLY — installer sequencing reference, nothing of it ships in the Core binary. The reference's questions are about one reference machine, and CrossOS's questions are the daemon's page schema.
CrossOS license: MIT

Source repository: raxigan/pcfy-my-mac
Source commit: 7ebef7f8df40a78b3c5761d10bceb1abaf96c1ce
Source file: cmd/param/param.go
Original license: MIT
Original copyright: Copyright (c) 2024 Raxigan
CrossOS destination: app/frontend/src/controls/WizardControl.tsx (the rule that a step the daemon has already derived as done is not asked again)
Modification: structural port, no code copied. `CollectSurveyParams` deletes from the question list every question the config file has already answered (param.go:120-138), and probes the machine for installed IDEs to append one more question (param.go:140-156), so a decision the machine has recorded is never re-asked. The wizard's equivalent is that a step whose verdict arrives from the daemon's onboarding row carries a Done mark and is never re-derived by the shell — the step machine is derived from live state (core/cmd/crossos/pagedata.go, handleOnboardingState), and this control renders those verdicts rather than keeping a cursor of its own. The reference's `FileParams`/`findIdes` probing and its `slices.DeleteFunc` call are not ported: CrossOS asks the daemon, and a question the daemon has answered is a step marked done.
Reason for modification: language and medium port (a CLI survey that filters its own questions becomes a declared-schema renderer that renders a daemon-derived step machine); the reference filters Go structs it read from a file, and CrossOS filters nothing — the daemon does the deriving and the shell draws the answer.
CrossOS license: MIT

Source repository: raxigan/pcfy-my-mac
Source commit: 7ebef7f8df40a78b3c5761d10bceb1abaf96c1ce
Source file: cmd/common/await.go
Original license: MIT
Original copyright: Copyright (c) 2024 Raxigan
CrossOS destination: app/frontend/src/controls/WizardControl.tsx (the bounded wait on the flow's last step: a 1000ms poll for up to 60s)
Modification: structural port, no code copied. `Until(condition, WithPollInterval, WithTimeout)` (await.go:30-68) is a ticker that re-runs a predicate until it holds or a context deadline expires, and the failure it returns names the budget it waited (`fmt.Errorf("%w after %v", ErrTimeout, cfg.Timeout)`). The wizard's last step is that shape: it re-reads the readiness verb on a timer, stops the moment every row the daemon sent is ready, stops at a stated deadline, and says which. What transfers is the SHAPE and the two NUMBERS — the reference's own call site uses 1000ms and 60s (task.go:60-67), and those are the values CrossOS ships. The reference's `context.WithTimeout`, `time.NewTicker` and `Config`/`WithPollInterval`/`WithTimeout` options are not ported: this is a React effect with a `setTimeout` chain and a `live` flag for unmount, which the reference has no need of because its process exits. The poll is a WAIT and never a verdict — it produces no readiness mark of its own, and on success it re-reads the daemon's own row so the step verdicts come from the daemon as before.
Reason for modification: language and medium port (a blocking Go ticker becomes an async React hook), and §9.11 records pcfy-my-mac as SCRIPTS ONLY — installer sequencing reference, nothing of it ships in the Core binary. The reference waits for a file it just wrote to appear on disk, and CrossOS waits for a tap the person just granted in System Settings to reinstall — the same gap between a command returning and the machine catching up, which is why one read is not enough in either.
CrossOS license: MIT

Source repository: raxigan/pcfy-my-mac
Source commit: 7ebef7f8df40a78b3c5761d10bceb1abaf96c1ce
Source file: cmd/task/task.go
Original license: MIT
Original copyright: Copyright (c) 2024 Raxigan
CrossOS destination: app/frontend/src/controls/WizardControl.tsx (the settled wait shown as progress while it runs, and as a named timeout when it runs out)
Modification: structural port, no code copied. The reference runs a command, calls `i.Progress()` on every poll tick so a spinner is visibly moving (task.go:56-73, with the spinner itself at install/util.go:85-105), and on exhaustion logs `Timed out: <err> <task>` rather than continuing. The wizard's last step does the same three things: it prints the poll count and the budget while the wait runs, and when the budget elapses it says the wait ran out and explicitly distinguishes that from an answer about the machine. The two values used are the reference's own (1000ms interval, 60s budget) and the reference's `i.Progress()` and `log.Fatalf` are not ported — there is no spinner to drive and the shell never kills the process.
Reason for modification: language and medium port (a CLI installer's progress line and fatal log become a settings row and an in-place error); the reference's failure is fatal because the installer owns the terminal, whereas the shell's is a message on a window the person can keep working in.
CrossOS license: MIT

Source repository: raxigan/pcfy-my-mac
Source commit: 7ebef7f8df40a78b3c5761d10bceb1abaf96c1ce
Source file: cmd/launcher.go
Original license: MIT
Original copyright: Copyright (c) 2024 Raxigan
CrossOS destination: app/frontend/src/controls/WizardControl.tsx (the closing "Almost ready!" hand-off, and the list of what is still to grant)
Modification: structural port, no code copied. After the last task returns, `Install` prints "Installed successfully", prints "PC'fied", then clears the screen and prints a numbered "Almost ready!" block whose second item names each app whose system permission the person still has to grant (launcher.go:68-80). The wizard's last step ends the same way: instead of a finish line it renders "Almost ready! Still to grant:" above the daemon's own unready readiness rows, each drawn by the checklist's row renderer so the sentence is the one the checklist gives a reader a screen away. The reference's apps (Karabiner-Elements, Alt-Tab, Rectangle) and its first item (restart the tools, then pick the new keymap) are NOT ported: CrossOS's list is whatever the daemon reports unready, and the System Settings path in each line is the daemon's own `detail` field — the shell composes no pane location of its own, because inventing one would be a second opinion about a permission only the daemon knows the state of.
Reason for modification: language and medium port (a CLI installer's closing hand-off becomes a settings row); the reference's insight is that an install whose last task is a person granting permissions is not finished when the commands return, and that is platform- and product-independent, so the shape carries over while the content is the daemon's.
CrossOS license: MIT

No pcfy-my-mac source is included; the LICENSE is recorded here for the
sequencing reference only.
