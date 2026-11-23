//go:build !windows

package winfw

import (
	"context"
	"errors"
	"testing"
)

func TestFixerOffWindowsCannotElevate(t *testing.T) {
	err := Fixer{Port: 631}.Apply(context.Background(), false)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}
