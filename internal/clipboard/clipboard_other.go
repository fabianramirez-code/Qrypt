//go:build !windows

package clipboard

func Copy(text string) error { return ErrUnavailable }

func ClearIfUnchanged(text string) error { return ErrUnavailable }
