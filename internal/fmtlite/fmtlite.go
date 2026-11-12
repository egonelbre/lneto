// Package fmtlite implements Format methods for errors without calling fmt,
// so that an error message can be appended to a buffer without building
// intermediate strings. fmt is imported only for the State and Formatter
// types, so that these Format methods are fmt Formatters. See
// https://go.dev/issue/83013.
package fmtlite

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

// State is fmt.State.
type State = fmt.State

// Formatter is fmt.Formatter.
type Formatter = fmt.Formatter

// FormatError implements Format for an error so that it prints exactly as
// fmt prints a type without a Format method, for the verbs fmt applies to
// an error's text (%v %s %q %x %X, with any flags, width and precision).
// write writes the error's message to the State it is given; bare %v and
// %s call it on s directly, so nothing is built in between. isNil reports
// a nil pointer receiver, which fmt prints as "<nil>".
//
// %#v and the other verbs print as "%!verb(message)": fmt renders those
// from the value by reflection, which fmtlite cannot do.
func FormatError(s State, verb rune, isNil bool, write func(State)) {
	_, hasWid := s.Width()
	_, hasPrec := s.Precision()
	switch {
	case verb == 'v' && s.Flag('#'), !isStringVerb(verb):
		WriteString(s, "%!")
		WriteString(s, string(verb))
		WriteString(s, "(")
		if isNil {
			WriteString(s, "<nil>")
		} else {
			write(s)
		}
		WriteString(s, ")")
	case isNil:
		WriteString(s, "<nil>")
	case (verb == 'v' || verb == 's') && !hasWid && !hasPrec:
		write(s)
	default:
		var b Buffer // Escapes, but only these rarer verbs reach it.
		write(&b)
		FormatString(s, verb, string(b))
	}
}

func isStringVerb(verb rune) bool {
	switch verb {
	case 'v', 's', 'x', 'X', 'q':
		return true
	}
	return false
}

// Print writes err as fmt's %v would. An inner Formatter writes straight
// into s; it is called with 's' so a '#' flag on s cannot turn it into %#v.
func Print(s State, err error) {
	switch e := err.(type) {
	case nil:
		WriteString(s, "<nil>")
	case Formatter:
		e.Format(s, 's')
	default:
		WriteString(s, e.Error())
	}
}

// WriteString writes str to s without converting it to []byte when s
// has a WriteString method, as fmt's State and Buffer do.
func WriteString(s State, str string) {
	if sw, ok := s.(interface{ WriteString(string) (int, error) }); ok {
		sw.WriteString(str)
		return
	}
	s.Write([]byte(str))
}

// FormatString formats str for verb v, s, x, X or q, honouring s's flags,
// width and precision. It follows fmt's string formatting.
func FormatString(s State, verb rune, str string) {
	switch verb {
	case 'v', 's':
		pad(s, truncate(s, str))
	case 'q':
		str = truncate(s, str)
		if s.Flag('#') && strconv.CanBackquote(str) {
			pad(s, "`"+str+"`")
		} else if s.Flag('+') {
			pad(s, strconv.QuoteToASCII(str))
		} else {
			pad(s, strconv.Quote(str))
		}
	case 'x':
		hex(s, str, "0123456789abcdefx")
	case 'X':
		hex(s, str, "0123456789ABCDEFX")
	}
}

// truncate keeps at most precision runes.
func truncate(s State, str string) string {
	if n, ok := s.Precision(); ok {
		for i := range str {
			n--
			if n < 0 {
				return str[:i]
			}
		}
	}
	return str
}

// pad writes str padded to the width in runes.
func pad(s State, str string) {
	w, _ := s.Width()
	n := w - utf8.RuneCountInString(str)
	if s.Flag('-') {
		WriteString(s, str)
		padding(s, n)
		return
	}
	padding(s, n)
	WriteString(s, str)
}

// padding writes n pad bytes: zeros only when padding on the left.
func padding(s State, n int) {
	const spaces, zeros = "                ", "0000000000000000"
	fill := spaces
	if s.Flag('0') && !s.Flag('-') {
		fill = zeros
	}
	for n > 0 {
		k := min(n, len(fill))
		WriteString(s, fill[:k])
		n -= k
	}
}

// hex writes str hex encoded as fmt's %x does; precision counts bytes.
func hex(s State, str, digits string) {
	length := len(str)
	if p, ok := s.Precision(); ok && p < length {
		length = p
	}
	sharp, space, minus := s.Flag('#'), s.Flag(' '), s.Flag('-')
	wid, hasWid := s.Width()
	width := 2 * length
	if width == 0 {
		if hasWid {
			padding(s, wid)
		}
		return
	}
	if space {
		if sharp {
			width *= 2
		}
		width += length - 1
	} else if sharp {
		width += 2
	}
	if hasWid && wid > width && !minus {
		padding(s, wid-width)
	}
	var buf [64]byte
	out := buf[:0]
	if sharp {
		out = append(out, '0', digits[16])
	}
	for i := 0; i < length; i++ {
		if space && i > 0 {
			out = append(out, ' ')
			if sharp {
				out = append(out, '0', digits[16])
			}
		}
		c := str[i]
		out = append(out, digits[c>>4], digits[c&0xF])
		if len(out) > len(buf)-8 {
			s.Write(out)
			out = buf[:0]
		}
	}
	s.Write(out)
	if hasWid && wid > width && minus {
		padding(s, wid-width)
	}
}

// Buffer is a State that appends to a byte slice. It has no flags, width
// or precision.
type Buffer []byte

func (b *Buffer) Write(p []byte) (int, error)       { *b = append(*b, p...); return len(p), nil }
func (b *Buffer) WriteString(s string) (int, error) { *b = append(*b, s...); return len(s), nil }
func (b *Buffer) Width() (int, bool)                { return 0, false }
func (b *Buffer) Precision() (int, bool)            { return 0, false }
func (b *Buffer) Flag(int) bool                     { return false }

// Append appends err's message to dst. A Formatter writes straight into
// dst through b, so nothing is built in between. b must outlive the call
// cheaply, such as a struct field: a pointer to a local Buffer escapes
// through the Format call and costs an allocation.
func Append(b *Buffer, dst []byte, err error) []byte {
	f, ok := err.(Formatter)
	if !ok {
		if err == nil {
			return append(dst, "<nil>"...)
		}
		return append(dst, err.Error()...)
	}
	*b = dst
	f.Format(b, 'v')
	dst = *b
	*b = nil // Do not keep dst alive.
	return dst
}

// Sprint implements an Error method on top of Format. The first 64 bytes
// live in the same allocation as the State, so a typical message costs
// two allocations: the State and the string.
func Sprint(f Formatter) string {
	sb := new(struct {
		b   Buffer
		arr [64]byte
	})
	sb.b = sb.arr[:0]
	f.Format(&sb.b, 'v')
	return string(sb.b)
}
