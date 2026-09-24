// Reading the daemon's untyped payloads, and the words a rejection is shown in
// (bead cross-os-itq).
//
// Two of the shell's rows do not arrive typed. config.getShortcuts is a Go
// []map[string]any, and a plugin's config_schema is whatever JSON Schema that
// plugin's manifest declared — the shell has no way to know its properties in
// advance, which is the whole point of §3.6c discovery. Every read of either
// goes through a narrowing function here, so an absent or oddly-typed field
// renders as "not reported" instead of `undefined` leaking into the page as
// text. A settings pane that prints "undefined" four times has told the user
// nothing and looks broken; a pane that says "not reported" has told them the
// truth about what the daemon knows.

/** A string field, or a fallback when the field is absent or another type. */
export function asText(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

/** A finite number, or null. JSON has no NaN, so a non-finite value is a
 *  decode failure the shell must not pass on to a number input. */
export function asNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

/** An array field, or an empty list — never null, matching the wire contract
 *  that collections cross as []. */
export function asList(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

/** An object field, or null. The value is returned AS-IS — it is the same
 *  object inside page.Schema or control.schema — so a caller must not mutate
 *  it: the payload another control is reading would change underneath them. A
 *  shallow copy would not buy isolation either, since the nested values would
 *  still be shared. */
export function asRecord(value: unknown): Record<string, unknown> | null {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

/** A boolean field, or null. `checked={asBool(x) ?? false}` is NOT the same as
 *  x being false, which is why this returns null instead of defaulting. */
export function asBool(value: unknown): boolean | null {
  return typeof value === 'boolean' ? value : null
}

/** Splits a modifier list typed as "ctrl, shift" into the []string the daemon
 *  stores. An empty entry is dropped so a trailing comma is not a modifier. */
export function splitModifiers(text: string): string[] {
  return text
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part !== '')
}

/**
 * describeError unwraps whatever the bridge rejected with. Wails surfaces a Go
 * error as a plain string, a transport failure as an Error, and a dropped
 * bridge as undefined — all three have to read as a sentence, because the
 * error row is the only place a user will ever learn why their change did not
 * stick.
 */
export function describeError(err: unknown): string {
  if (typeof err === 'string' && err !== '') return err
  if (err instanceof Error && err.message !== '') return err.message
  if (err !== null && typeof err === 'object' && 'message' in err) {
    const message = (err as { message: unknown }).message
    if (typeof message === 'string' && message !== '') return message
  }
  return 'the shell got no answer from the daemon'
}

/**
 * failedTo phrases a rejected call so the reader knows both what they tried and
 * what stopped it. The daemon's own message is kept verbatim: it is the only
 * part that names the rule that rejected the edit ("duplicate chord …"), and
 * paraphrasing that would throw away the one clue that makes it fixable.
 */
export function failedTo(action: string, err: unknown): string {
  return `${action} did not go through: ${describeError(err)}`
}

/**
 * describeResult renders what a command returned so a button never reports
 * "done" and nothing else. safety.panicStop in particular returns the list of
 * what it stopped (bridge.go: "Result names what stopped — never a silent
 * kill"), and flattening that into a success word would throw the only useful
 * part of the answer away.
 */
export function describeResult(value: unknown): string {
  if (value === null || value === undefined) return 'done'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  const record = asRecord(value)
  if (record) {
    const pairs = Object.entries(record)
      .filter(([, entry]) => entry !== null && entry !== undefined && entry !== '')
      .map(([key, entry]) => `${key}: ${Array.isArray(entry) ? entry.join(', ') : String(entry)}`)
    return pairs.length > 0 ? pairs.join(' · ') : 'done'
  }
  if (Array.isArray(value)) {
    const items = value.map((entry) => (typeof entry === 'string' ? entry : JSON.stringify(entry)))
    return items.length > 0 ? items.join(' → ') : 'done'
  }
  return String(value)
}
