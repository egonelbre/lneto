package lneto

import (
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/soypat/lneto/internal/fmtlite"
)

var (
	_ fmt.Formatter = (*BitPosErr)(nil)
	_ fmt.Formatter = (*joinError)(nil)
)

func TestValidatorErrPopJoined(t *testing.T) {
	bpe := &BitPosErr{BitStart: 8, BitLen: 8, Err: ErrInvalidField}
	var v Validator
	v.flags |= validateAllowMultiErrors
	v.AddError(ErrBadCRC)
	v.AddError(bpe)
	v.AddError(ErrShortBuffer)
	err := v.ErrPop()
	want := errors.Join(ErrBadCRC, bpe, ErrShortBuffer)

	if got := err.Error(); got != want.Error() {
		t.Fatalf("Error() = %q, want %q", got, want.Error())
	}
	for _, target := range []error{ErrBadCRC, bpe, ErrShortBuffer} {
		if !errors.Is(err, target) {
			t.Errorf("errors.Is(err, %v) = false", target)
		}
	}
	for _, d := range []string{"%v", "%q", "%x", "%20.7s"} {
		if got, want := fmt.Sprintf(d, err), fmt.Sprintf(d, want); got != want {
			t.Errorf("%q: got %q, want %q", d, got, want)
		}
	}
	if v.HasError() {
		t.Error("ErrPop did not clear the validator")
	}
	var b fmtlite.Buffer
	dst := make([]byte, 0, 128)
	if got := string(fmtlite.Append(&b, dst, err)); got != want.Error() {
		t.Errorf("Append = %q, want %q", got, want.Error())
	}
	if runtime.Compiler == "tinygo" { // TinyGo's testing.AllocsPerRun does not count allocations.
		return
	}
	if allocs := testing.AllocsPerRun(100, func() { dst = fmtlite.Append(&b, dst[:0], err) }); allocs != 0 {
		t.Errorf("Append allocs = %v, want 0", allocs)
	}
}
