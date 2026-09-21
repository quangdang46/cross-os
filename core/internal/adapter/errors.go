package adapter

import "errors"

// errUnsupported marks seams with no implementation on this platform.
var errUnsupported = errors.New("adapter: not supported on this platform")
