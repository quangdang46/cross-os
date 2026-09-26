// Preview-only replacement for src/lib/service: the same bound surface, served
// by the test fixture's stateful machine, so the shell renders real pages
// without the Wails bridge.
import { stub } from '../src/test/fixtures'

export const Service = stub as never
export const service = stub as never
