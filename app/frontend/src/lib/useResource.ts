// The one data-loading hook the control renderers share (bead cross-os-itq).
//
// It exists to enforce two rules that are easy to break one control at a time:
//
//   - The daemon is asked on mount and when ControlContext.refreshToken
//     changes, and at no other time. App.tsx owns the refresh cadence; a
//     control that opened its own interval would multiply every page's daemon
//     calls by its own poll rate, and the first page with three list controls
//     on it would triple the load for no new information.
//
//   - A load that is already in flight when the token changes is abandoned.
//     The daemon answers over a Unix socket that can stall; without the
//     generation check a slow answer to an old question lands after a fast
//     answer to a new one and the page shows stale data as if it were current.

import { useCallback, useEffect, useRef, useState } from 'react'
import { describeError } from './wire'

export interface Resource<T> {
  data: T
  /** The last load failure, kept for as long as it lasts. */
  error: string
  loading: boolean
  /** Re-runs the load without waiting for the next refresh token. */
  reload: () => void
}

export function useResource<T>(
  load: () => Promise<T>,
  version: number,
  initial: T,
  announce?: (message: string) => void,
): Resource<T> {
  const [data, setData] = useState<T>(initial)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [attempt, setAttempt] = useState(0)

  const generation = useRef(0)
  // The loader and the notifier are re-created every render by their callers
  // (they close over ctx), so they are read through refs: a control that
  // rebuilt its callback each pass would otherwise restart the effect on every
  // render and fetch in a loop.
  const loader = useRef(load)
  loader.current = load
  const notifier = useRef(announce)
  notifier.current = announce
  // App.tsx re-reads every few seconds. Without this the same daemon failure
  // would reprint its note on every tick and push the user's own messages off
  // the screen, so ctx.note hears about a given failure once and the control's
  // own error row keeps showing it until it is fixed.
  const announced = useRef('')

  useEffect(() => {
    const mine = ++generation.current
    setLoading(true)
    loader.current().then(
      (value) => {
        if (mine !== generation.current) return
        setData(value)
        setError('')
        setLoading(false)
      },
      (reason: unknown) => {
        if (mine !== generation.current) return
        const message = describeError(reason)
        setError(message)
        setLoading(false)
        if (message !== announced.current) {
          announced.current = message
          notifier.current?.(message)
        }
      },
    )
  }, [version, attempt])

  const reload = useCallback(() => setAttempt((n) => n + 1), [])
  return { data, error, loading, reload }
}
