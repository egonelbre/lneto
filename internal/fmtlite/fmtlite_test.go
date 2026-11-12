package fmtlite_test

import (
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/soypat/lneto/internal/fmtlite"
)

// prefixErr is an error that formats through fmtlite.
type prefixErr struct {
	prefix string
	err    error
}

func (e *prefixErr) Error() string { return fmtlite.Sprint(e) }
func (e *prefixErr) Format(s fmt.State, verb rune) {
	fmtlite.FormatError(s, verb, e == nil, func(s fmtlite.State) {
		fmtlite.WriteString(s, e.prefix)
		fmtlite.Print(s, e.err)
	})
}

// refErr has the same message, built by concatenation, and no Format.
type refErr struct {
	prefix string
	err    error
}

func (e *refErr) Error() string { return e.prefix + e.err.Error() }

func TestFormatErrorMatchesFmt(t *testing.T) {
	directives := []string{"%v", "%s", "%+v", "%#s", "%q", "%#q", "%+q", "%x", "%X",
		"% x", "%#x", "%# X", "%.2x", "%3.1x", "%30v", "%-30v|", "%05s", "%-05s|",
		"%08q", "%.5s", "%.5v", "%30.5q", "[%v]", "%[1]v %[1]q"}
	eof := errors.New("EOF")
	tests := []struct {
		name string
		got  error
		want error
	}{
		{name: "plain inner", got: &prefixErr{"cfg: ", eof}, want: &refErr{"cfg: ", eof}},
		{name: "nested", got: &prefixErr{"a: ", &prefixErr{"b: ", eof}}, want: &refErr{"a: ", &refErr{"b: ", eof}}},
		{name: "unicode", got: &prefixErr{"öpen /ä/日本: ", eof}, want: &refErr{"öpen /ä/日本: ", eof}},
		{name: "empty", got: &prefixErr{"", errors.New("")}, want: &refErr{"", errors.New("")}},
		{name: "nil receiver", got: (*prefixErr)(nil), want: (*refErr)(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, d := range directives {
				if got, want := fmt.Sprintf(d, tt.got), fmt.Sprintf(d, tt.want); got != want {
					t.Errorf("%q: got %q, want %q", d, got, want)
				}
			}
		})
	}
	// fmt renders these from the value by reflection; fmtlite cannot.
	for d, want := range map[string]string{"%#v": "%!v(p: EOF)", "%d": "%!d(p: EOF)"} {
		if got := fmt.Sprintf(d, &prefixErr{"p: ", eof}); got != want {
			t.Errorf("%q: got %q, want %q", d, got, want)
		}
	}
}

func TestAppend(t *testing.T) {
	eof := errors.New("EOF")
	chain := &prefixErr{"a: ", &prefixErr{"b: ", &prefixErr{"c: ", eof}}}
	var b fmtlite.Buffer
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "formatter", err: chain, want: "x=a: b: c: EOF"},
		{name: "plain", err: eof, want: "x=EOF"},
		{name: "nil", err: nil, want: "x=<nil>"},
	}
	for _, tt := range tests {
		if got := string(fmtlite.Append(&b, []byte("x="), tt.err)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
	if b != nil {
		t.Error("Append kept a reference to dst")
	}
	if got := chain.Error(); got != "a: b: c: EOF" {
		t.Errorf("Sprint: got %q", got)
	}
	// TinyGo's testing.AllocsPerRun does not count allocations.
	if runtime.Compiler == "tinygo" {
		return
	}
	dst := make([]byte, 0, 64)
	if allocs := testing.AllocsPerRun(100, func() { dst = fmtlite.Append(&b, dst[:0], chain) }); allocs != 0 {
		t.Errorf("Append allocs = %v, want 0", allocs)
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = chain.Error() }); allocs != 2 {
		t.Errorf("Sprint allocs = %v, want 2", allocs)
	}
}
