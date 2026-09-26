// A control kind this build has no renderer for (bead cross-os-itq).
//
// The three Finder-page kinds (packList, actionSettings, gateBadge) land here
// today, and so would any kind a future page invents. That is acceptable only
// because this row NAMES the kind and says why it is empty — the rule is that
// a missing renderer is a defect someone can find, not a page that renders
// blank and leaves the user wondering whether CrossOS is broken.
//
// The fix is a renderer and one entry in the kind registry (controls/index.tsx),
// never a branch in App.tsx: a shell that special-cased a kind by name would
// make the next page a UI change, which is the rule §3.6c exists to prevent.
//
// It is deliberately not styled as an error. Nothing failed at runtime; the
// shell simply cannot draw what the page declared, and a row in the error style
// would send users looking for a permission problem that does not exist.
//
// THE THREE LINES ARE THE REFERENCE'S EMPTY STATE, and the order is the port.
// menumate's ScreenPacksEmpty (App/UI/PacksScreen.swift:372-405) is a title, a
// body and an action, in three voices and never welded together: the title says
// the state in the primary label at 17pt semibold ("No packs yet"), the body says
// it in the second tone at 12.5pt with 2.5 line spacing ("Browse community packs
// or import a Git repository."), and the action is a pair of buttons under both
// (:396-399). One sentence doing all three jobs is what produced the sentence
// this file used to have, which had to carry the kind, the reason and the
// consequence in a clause and a half.
//
// The middle line is the reference's DestructiveRestoreDialog impact sentence
// (GeneralTab.swift:197-230) doing the one job it does there: naming what is NOT
// affected, so a reader knows the blast radius. Here that is "nothing else on
// this page failed" — which is the fact, and is the same reassurance
// restorePresets' "Custom actions and extension packs are unaffected" gives
// before a destructive confirm.
//
// There is no button, and the reference has the same gap: onboarding's
// `diag.extNotEnabled` is a state with no action because there was no door to
// point at (App/Localizable.xcstrings, "Finder extension not yet enabled"). The
// door here is a renderer and one entry in the kind registry, which is a change
// to the build and not a thing a reader can press, so it is named in words and
// inventing a button for it would be a button that runs nothing.

import type { ReactElement } from 'react'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function UnsupportedControl(props: ControlProps): ReactElement {
  const { control } = props
  const label = control.label ?? control.id
  const kind = control.kind || 'unnamed'

  return (
    <ControlFrame label={label} note={control.note}>
      <p className="ctl-value">
        Declared kind: <span className="ctl-chip">{control.kind || '(none)'}</span>
      </p>
      <EmptyState>
        {label} is a “{kind}” control, and this build has no renderer for that kind, so there is
        nothing to draw here. Nothing else on this page is affected.
      </EmptyState>
    </ControlFrame>
  )
}
