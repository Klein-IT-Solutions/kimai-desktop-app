package desktop

import "errors"

// ErrUnsupported is returned where a feature is not implemented for the OS.
var ErrUnsupported = errors.New("not supported on this platform")
