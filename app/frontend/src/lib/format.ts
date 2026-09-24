// Display formatters shared by the control renderers (bead cross-os-itq).
//
// Every function here is display-only: it takes a value the daemon already sent
// and makes it readable for someone who is not reading the plan. None of them
// decides what is true, and a caller that hides the raw value must keep it
// reachable (a title attribute), because a formatter that guesses wrong would
// quietly misreport the user's own configuration.
//
// §1 of the plan is explicit that CrossOS is for people who are not reading the
// plan; "leftHalf" and "core:licenseMIT" are daemon vocabulary, not English.

const MODIFIER_ALIASES: Record<string, string> = {
  command: 'cmd',
  cmd: 'cmd',
  super: 'cmd',
  meta: 'cmd',
  win: 'cmd',
  control: 'ctrl',
  ctrl: 'ctrl',
  option: 'alt',
  opt: 'alt',
  alt: 'alt',
  shift: 'shift',
}

/**
 * formatChord renders a rule's key chord ("control + shift + K") the way a
 * settings pane does: short modifier names, capital single keys, '+' joined.
 * A token this shell does not recognise is passed through untouched rather
 * than dropped — an unknown modifier is still a modifier the user pressed, and
 * silently losing it would show them the wrong shortcut.
 */
export function formatChord(keys: string): string {
  const parts = keys
    .split('+')
    .map((part) => part.trim())
    .filter((part) => part !== '')
  if (parts.length === 0) return ''
  return parts
    .map((part) => {
      const alias = MODIFIER_ALIASES[part.toLowerCase()]
      if (alias) return alias
      return part.length === 1 ? part.toUpperCase() : part
    })
    .join('+')
}

/**
 * humanize turns a daemon identifier into something a person can read
 * ("leftHalf" → "Left half", "windows-keyboard" → "Windows keyboard"). Used for
 * zone names, plugin ids and readiness ids, all of which are machine names the
 * daemon chose. Anything that does not look like an identifier is returned
 * unchanged, so a title the user wrote is never reflowed behind their back.
 */
export function humanize(id: string): string {
  const trimmed = id.trim()
  if (trimmed === '') return ''
  const spaced = trimmed
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
  if (spaced === trimmed && !/[ _-]/.test(trimmed)) return trimmed
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

/**
 * plural keeps a count grammatical without pulling an i18n dependency into a
 * shell that is not allowed a new npm package for one word.
 */
export function plural(count: number, one: string, many?: string): string {
  return `${count} ${count === 1 ? one : (many ?? `${one}s`)}`
}

/**
 * relativeTime renders an audit timestamp as "3 minutes ago". An absent or
 * unparseable value returns text that says so, or the raw string — never
 * "just now", because claiming a freshness the shell cannot support is the
 * kind of small lie that makes an audit list untrustworthy.
 */
export function relativeTime(stamp: string, now: number = Date.now()): string {
  if (!stamp) return 'time not reported'
  const at = Date.parse(stamp)
  if (Number.isNaN(at)) return stamp
  const seconds = Math.round((now - at) / 1000)
  if (seconds < 5) return 'just now'
  if (seconds < 60) return `${plural(seconds, 'second')} ago`
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `${plural(minutes, 'minute')} ago`
  const hours = Math.round(minutes / 60)
  if (hours < 24) return `${plural(hours, 'hour')} ago`
  return `${plural(Math.round(hours / 24), 'day')} ago`
}

/**
 * formatCountdown renders a trial's remaining milliseconds as m:ss. A lapsed
 * trial reads "0:00" rather than a negative time; what zero MEANS is the
 * daemon's call, and the display only reports the number it was handed.
 */
export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const minutes = Math.floor(total / 60)
  const seconds = total % 60
  return `${minutes}:${seconds < 10 ? `0${seconds}` : seconds}`
}

/**
 * splitLogSource splits one log line into the part that names the call and the
 * part that says what happened ("GetMatrix: daemon unreachable" →
 * {source, detail}). These lines are display strings, not a structured payload,
 * so the split is best-effort by design: a line without that shape comes back
 * whole rather than cut at a guessed place, which is what a timeline wants — an
 * opaque line it can still show beats a line it shows wrong.
 */
export function splitLogSource(line: string): { source: string; detail: string } {
  const cut = line.indexOf(': ')
  if (cut <= 0) return { source: '', detail: line }
  return { source: line.slice(0, cut), detail: line.slice(cut + 2) }
}
