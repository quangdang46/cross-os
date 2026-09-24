// The kind earlier builds of the daemon served for the activity feed.
//
// `traceList` is declared by the Activity page (ActivityPage, pages.go) and the
// Observe page (ObservePage, pages.go), and App.tsx tests for that spelling to
// decide whether the page already draws a log. Both now land on the pipeline
// renderer, which reads the daemon's structured traces (core:traces) and shows
// each decision's stages as their own rows — so this file is a NAME, not a
// second renderer: one implementation, two spellings of one kind.
//
// The alias keeps a daemon that has not been rebuilt alongside the shell from
// dropping two pages into UnsupportedControl. Retiring the spelling is a Go
// change, and App.tsx's `ownsTrace` test would have to move with it.

export { PipelineTraceControl as TraceListControl } from './PipelineTraceControl'
