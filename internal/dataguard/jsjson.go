package dataguard

import (
	"fmt"
	"strings"

	"github.com/panphora/htmlclay/internal/dataapi"
)

// Stringify writes exactly what JSON.stringify writes for an extracted value.
func Stringify(v dataapi.Value) string {
	var b strings.Builder
	writeValue(&b, v)
	return b.String()
}

func writeValue(b *strings.Builder, v dataapi.Value) {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case string:
		writeString(b, t)
	case []dataapi.Value:
		b.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			writeValue(b, item)
		}
		b.WriteByte(']')
	case *dataapi.Object:
		b.WriteByte('{')
		for i, k := range jsKeys(t) {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, k)
			b.WriteByte(':')
			val, _ := t.Get(k)
			writeValue(b, val)
		}
		b.WriteByte('}')
	}
}

func writeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
