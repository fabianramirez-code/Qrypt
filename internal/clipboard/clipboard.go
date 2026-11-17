// Package clipboard provides native clipboard access for ephemeral text.
package clipboard

import "errors"

var ErrUnavailable = errors.New("clipboard: native clipboard not available")
