package stencil

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Values are nil, string, bool, int, float64, time.Time, a TimeValue,
// []any or map[string]any. The planet and article contexts are maps keyed
// by their JSON names, which is how Stencil sees Planet's structs too.

// TimeValue is a date that knows its own seconds as Foundation holds
// them, so a template printing article.created.timeIntervalSince1970
// gets the same double arithmetic Swift does.
type TimeValue interface {
	Time() time.Time
	IntervalSince1970() float64
	IntervalSinceReferenceDate() float64
}

// Filter transforms a value, with the arguments written after the colon.
type Filter func(value any, args []any) (any, error)

// Context is the variable stack a template renders against.
type Context struct {
	env    *Env
	stack  []map[string]any
	blocks *blockContext
}

func (c *Context) push(m map[string]any) { c.stack = append(c.stack, m) }
func (c *Context) pop()                  { c.stack = c.stack[:len(c.stack)-1] }

func (c *Context) lookup(key string) (any, bool) {
	for i := len(c.stack) - 1; i >= 0; i-- {
		if v, ok := c.stack[i][key]; ok {
			return v, true
		}
	}
	return nil, false
}

// variable is a key path with filters: `article.title|escape`.
type variable struct {
	raw     string
	literal any
	isLit   bool
	path    []pathPart
	filters []filterCall
}

type pathPart struct {
	key string
	sub *variable // a bracket subscript resolved at render time
}

type filterCall struct {
	name string
	args []*variable
}

func parseVariable(s string) (*variable, error) {
	bits := smartSplit(s, '|')
	v := &variable{raw: s}
	head := strings.TrimSpace(bits[0])
	for _, f := range bits[1:] {
		name, args := parseFilterComponents(f)
		fc := filterCall{name: name}
		for _, a := range args {
			av, err := parseVariable(a)
			if err != nil {
				return nil, err
			}
			fc.args = append(fc.args, av)
		}
		v.filters = append(v.filters, fc)
	}
	if len(head) > 1 && (head[0] == '"' && head[len(head)-1] == '"' || head[0] == '\'' && head[len(head)-1] == '\'') {
		v.isLit, v.literal = true, head[1:len(head)-1]
		return v, nil
	}
	if n, err := strconv.ParseInt(head, 10, 64); err == nil {
		v.isLit, v.literal = true, int(n)
		return v, nil
	}
	if f, err := strconv.ParseFloat(head, 64); err == nil && strings.ContainsAny(head, ".eE") {
		v.isLit, v.literal = true, f
		return v, nil
	}
	if head == "true" || head == "false" {
		v.isLit, v.literal = true, head == "true"
		return v, nil
	}
	parts, err := parseKeyPath(head)
	if err != nil {
		return nil, err
	}
	v.path = parts
	return v, nil
}

func parseFilterComponents(s string) (string, []string) {
	comps := smartSplit(s, ':')
	name := strings.Trim(comps[0], " ")
	var args []string
	if len(comps) > 1 {
		for _, a := range smartSplit(strings.Join(comps[1:], ":"), ',') {
			args = append(args, strings.Trim(a, " "))
		}
	}
	return name, args
}

// parseKeyPath reads `a.b['k'][0].c` into parts.
func parseKeyPath(s string) ([]pathPart, error) {
	var parts []pathPart
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, pathPart{key: cur.String()})
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.':
			flush()
		case '[':
			flush()
			end := strings.IndexByte(s[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("unclosed [ in %q", s)
			}
			inner := strings.TrimSpace(s[i+1 : i+end])
			sub, err := parseVariable(inner)
			if err != nil {
				return nil, err
			}
			if sub.isLit {
				parts = append(parts, pathPart{key: stringify(sub.literal)})
			} else {
				parts = append(parts, pathPart{sub: sub})
			}
			i += end
		default:
			cur.WriteByte(s[i])
		}
	}
	flush()
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty variable")
	}
	return parts, nil
}

func (v *variable) resolve(c *Context) (any, error) {
	val, err := v.resolveBare(c)
	if err != nil {
		return nil, err
	}
	for _, f := range v.filters {
		fn, ok := c.env.filters[f.name]
		if !ok {
			return nil, fmt.Errorf("unknown filter %q", f.name)
		}
		var args []any
		for _, a := range f.args {
			av, err := a.resolve(c)
			if err != nil {
				return nil, err
			}
			args = append(args, av)
		}
		val, err = fn(val, args)
		if err != nil {
			return nil, fmt.Errorf("filter %s: %w", f.name, err)
		}
	}
	return val, nil
}

func (v *variable) resolveBare(c *Context) (any, error) {
	if v.isLit {
		return v.literal, nil
	}
	var cur any
	for i, p := range v.path {
		key := p.key
		if p.sub != nil {
			kv, err := p.sub.resolve(c)
			if err != nil {
				return nil, err
			}
			key = stringify(kv)
		}
		if i == 0 {
			val, ok := c.lookup(key)
			if !ok {
				return nil, nil
			}
			cur = val
			continue
		}
		cur = lookupIn(cur, key)
		if cur == nil {
			return nil, nil
		}
	}
	return cur, nil
}

// lookupIn descends one step, with Stencil's collection words: count on
// maps, strings and arrays; first, last and an index on strings and arrays.
func lookupIn(v any, key string) any {
	switch x := v.(type) {
	case pair:
		switch key {
		case "key", "0":
			return x.key
		case "value", "1":
			return x.value
		}
		return nil
	case map[string]any:
		if key == "count" {
			if _, has := x["count"]; !has {
				return len(x)
			}
		}
		return x[key]
	case []any:
		return collectionWord(key, len(x), func(i int) any { return x[i] })
	case string:
		// Swift's String is a collection of grapheme clusters, so count,
		// first, last and an index all work on clusters.
		g := graphemes(x)
		return collectionWord(key, len(g), func(i int) any { return g[i] })
	case TimeValue:
		// Stencil reaches a Date's properties through Key-Value Coding on
		// the bridged NSDate, which answers with an NSNumber.
		switch key {
		case "timeIntervalSince1970":
			return nsNumber(x.IntervalSince1970())
		case "timeIntervalSinceReferenceDate":
			return nsNumber(x.IntervalSinceReferenceDate())
		}
	case time.Time:
		switch key {
		case "timeIntervalSince1970":
			return nsNumber(float64(x.UnixMicro()) / 1e6)
		case "timeIntervalSinceReferenceDate":
			return nsNumber(float64(x.UnixMicro()-978307200*1e6) / 1e6)
		}
	}
	return nil
}

func collectionWord(key string, n int, at func(int) any) any {
	switch key {
	case "count":
		return n
	case "first":
		if n > 0 {
			return at(0)
		}
		return nil
	case "last":
		if n > 0 {
			return at(n - 1)
		}
		return nil
	}
	if i, err := strconv.Atoi(key); err == nil && i >= 0 && i < n {
		return at(i)
	}
	return nil
}

// nsNumber is a number that came back from Key-Value Coding, as a Date's
// timeIntervalSince1970 does: Stencil prints it through NSNumber's
// description, which shows sixteen significant digits, not Swift's
// shortest round trip.
type nsNumber float64

// pair is a dictionary entry as a for loop sees it: Swift's (key, value)
// tuple, reachable as .key and .value or .0 and .1, and printing as
// nothing on its own, as a tuple does in Stencil.
type pair struct {
	key   string
	value any
}

// stringify prints a value as Stencil does: strings bare, numbers and
// booleans in Swift's description form, a date as Foundation prints it,
// an array as Swift's array description, nil as nothing.
func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return swiftDouble(x)
	case nsNumber:
		return strconv.FormatFloat(float64(x), 'g', 16, 64)
	case time.Time:
		return x.UTC().Format("2006-01-02 15:04:05 +0000")
	case TimeValue:
		return x.Time().UTC().Format("2006-01-02 15:04:05 +0000")
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = describe(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		if len(x) == 0 {
			return "[:]"
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = strconv.Quote(k) + ": " + describe(x[k])
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return fmt.Sprint(v)
}

// describe is stringify inside a collection, where Swift quotes strings.
func describe(v any) string {
	if s, ok := v.(string); ok {
		return strconv.Quote(s)
	}
	return stringify(v)
}

// swiftDouble prints a float64 as Swift's Double.description does: the
// shortest digits that round-trip, in plain decimal from 1e-4 up to 1e16
// with at least one digit after the point, and otherwise in exponent form.
func swiftDouble(f float64) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return fmt.Sprint(f)
	}
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	e := strconv.FormatFloat(f, 'e', -1, 64) // d.ddddde±XX
	mant, expStr, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expStr)
	neg := strings.HasPrefix(mant, "-")
	mant = strings.TrimPrefix(mant, "-")
	digits := strings.Replace(mant, ".", "", 1)
	var out string
	switch {
	case exp < -4 || exp >= 16:
		out = mant
		sign := "+"
		if exp < 0 {
			sign = "-"
			exp = -exp
		}
		out += fmt.Sprintf("e%s%02d", sign, exp)
	case exp < 0:
		out = "0." + strings.Repeat("0", -exp-1) + digits
	case exp+1 >= len(digits):
		out = digits + strings.Repeat("0", exp+1-len(digits)) + ".0"
	default:
		out = digits[:exp+1] + "." + digits[exp+1:]
	}
	if neg {
		out = "-" + out
	}
	return out
}

// truthy follows Stencil: non-empty collections and strings, true, and
// numbers above zero.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case bool:
		return x
	case string:
		return x != ""
	}
	if n, ok := toNumber(v); ok {
		return n > 0
	}
	return true
}

func toNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case nsNumber:
		return float64(x), true
	}
	return 0, false
}
