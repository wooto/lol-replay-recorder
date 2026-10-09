//go:build !windows || !amd64

package recorder

import (
	"errors"
	"testing"
)

func TestUnsupportedDesktop(t *testing.T) {
	if _, err := (nativeDesktop{}).acquire(); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatal(err)
	}
}
