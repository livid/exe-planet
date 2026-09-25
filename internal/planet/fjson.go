package planet

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"unicode/utf8"
)

// MarshalFoundation writes JSON the way Planet's shared JSONEncoder does:
// sorted keys, pretty printed with two spaces, a space on both sides of
// each colon, an empty array or object as its brackets around a blank
// line, slashes unescaped, non-ASCII kept as UTF-8. Values are string,
// bool, int, float64, Date, []string, []any, map[string]string,
// map[string][]string and map[string]any; nil is not written, so a
// caller omits an absent optional by leaving it out of the map.
func MarshalFoundation(v any) []byte {
	var b bytes.Buffer
	writeValue(&b, v, 0)
	return b.Bytes()
}

func indent(b *bytes.Buffer, n int) {
	for i := 0; i < n; i++ {
		b.WriteString("  ")
	}
}

func writeValue(b *bytes.Buffer, v any, depth int) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case string:
		writeString(b, x)
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case float64:
		b.WriteString(formatNumber(x))
	case Date:
		b.WriteString(x.String())
	case []string:
		items := make([]any, len(x))
		for i, s := range x {
			items[i] = s
		}
		writeArray(b, items, depth)
	case []any:
		writeArray(b, x, depth)
	case map[string]string:
		m := make(map[string]any, len(x))
		for k, s := range x {
			m[k] = s
		}
		writeObject(b, m, depth)
	case map[string][]string:
		m := make(map[string]any, len(x))
		for k, s := range x {
			m[k] = s
		}
		writeObject(b, m, depth)
	case map[string]any:
		writeObject(b, x, depth)
	default:
		panic(fmt.Sprintf("planet: cannot marshal %T", v))
	}
}

func writeArray(b *bytes.Buffer, items []any, depth int) {
	if len(items) == 0 {
		b.WriteString("[\n\n")
		indent(b, depth)
		b.WriteString("]")
		return
	}
	b.WriteString("[\n")
	for i, it := range items {
		indent(b, depth+1)
		writeValue(b, it, depth+1)
		if i < len(items)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	indent(b, depth)
	b.WriteString("]")
}

func writeObject(b *bytes.Buffer, m map[string]any, depth int) {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v != nil {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		b.WriteString("{\n\n")
		indent(b, depth)
		b.WriteString("}")
		return
	}
	sort.Strings(keys)
	b.WriteString("{\n")
	for i, k := range keys {
		indent(b, depth+1)
		writeString(b, k)
		b.WriteString(" : ")
		writeValue(b, m[k], depth+1)
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	indent(b, depth)
	b.WriteString("}")
}

func writeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c < 0x20:
			fmt.Fprintf(b, `\u%04x`, c)
		case c < utf8.RuneSelf:
			b.WriteByte(c)
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			b.WriteString(s[i : i+size])
			i += size
			continue
		}
		i++
	}
	b.WriteByte('"')
}
