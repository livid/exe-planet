package stencil

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Env is an environment: a loader for template names, the filters, and a
// cache of parsed templates.
type Env struct {
	loader  func(name string) (string, error)
	filters map[string]Filter
	mu      sync.Mutex
	cache   map[string]*Template
}

// Template is a parsed template.
type Template struct {
	name  string
	nodes []node
}

// NewEnv makes an environment whose templates come from loader, with the
// built-in filters registered.
func NewEnv(loader func(name string) (string, error)) *Env {
	e := &Env{loader: loader, filters: map[string]Filter{}, cache: map[string]*Template{}}
	registerBuiltins(e)
	return e
}

// DirLoader loads templates from a folder; a name is a path relative to
// it and may not climb out.
func DirLoader(dir string) func(string) (string, error) {
	return func(name string) (string, error) {
		clean := path.Clean("/" + strings.ReplaceAll(name, "\\", "/"))
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(clean)))
		if err != nil {
			return "", fmt.Errorf("template %s: %w", name, err)
		}
		return string(b), nil
	}
}

// RegisterFilter adds or replaces a filter.
func (e *Env) RegisterFilter(name string, f Filter) { e.filters[name] = f }

func (e *Env) template(name string) (*Template, error) {
	e.mu.Lock()
	t, ok := e.cache[name]
	e.mu.Unlock()
	if ok {
		return t, nil
	}
	if e.loader == nil {
		return nil, fmt.Errorf("template %s: no loader", name)
	}
	src, err := e.loader(name)
	if err != nil {
		return nil, err
	}
	t, err = e.Parse(name, src)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.cache[name] = t
	e.mu.Unlock()
	return t, nil
}

// Parse compiles a template string.
func (e *Env) Parse(name, src string) (*Template, error) {
	nodes, err := parse(name, src)
	if err != nil {
		return nil, err
	}
	return &Template{name: name, nodes: nodes}, nil
}

// Render renders the named template from the loader.
func (e *Env) Render(name string, ctx map[string]any) (string, error) {
	t, err := e.template(name)
	if err != nil {
		return "", err
	}
	return e.RenderTemplate(t, ctx)
}

// RenderString renders a template given as a string.
func (e *Env) RenderString(src string, ctx map[string]any) (string, error) {
	t, err := e.Parse("", src)
	if err != nil {
		return "", err
	}
	return e.RenderTemplate(t, ctx)
}

// RenderTemplate renders a parsed template.
func (e *Env) RenderTemplate(t *Template, ctx map[string]any) (string, error) {
	if ctx == nil {
		ctx = map[string]any{}
	}
	c := &Context{env: e, stack: []map[string]any{ctx}}
	var w strings.Builder
	if err := renderNodes(t.nodes, c, &w); err != nil {
		return "", fmt.Errorf("%s: %w", t.name, err)
	}
	return w.String(), nil
}

func sortStrings(s []string) { sort.Strings(s) }

// registerBuiltins installs Stencil's own filters.
func registerBuiltins(e *Env) {
	e.RegisterFilter("capitalize", func(v any, _ []any) (any, error) {
		s := stringify(v)
		if s == "" {
			return s, nil
		}
		return strings.ToUpper(s[:1]) + strings.ToLower(s[1:]), nil
	})
	e.RegisterFilter("uppercase", func(v any, _ []any) (any, error) { return strings.ToUpper(stringify(v)), nil })
	e.RegisterFilter("lowercase", func(v any, _ []any) (any, error) { return strings.ToLower(stringify(v)), nil })
	e.RegisterFilter("default", func(v any, args []any) (any, error) {
		if truthy(v) {
			return v, nil
		}
		for _, a := range args {
			if truthy(a) {
				return a, nil
			}
		}
		return nil, nil
	})
	e.RegisterFilter("join", func(v any, args []any) (any, error) {
		sep := ""
		if len(args) > 0 {
			sep = stringify(args[0])
		}
		items, _ := v.([]any)
		parts := make([]string, len(items))
		for i, it := range items {
			parts[i] = stringify(it)
		}
		return strings.Join(parts, sep), nil
	})
	e.RegisterFilter("split", func(v any, args []any) (any, error) {
		sep := " "
		if len(args) > 0 {
			sep = stringify(args[0])
		}
		var out []any
		for _, p := range strings.Split(stringify(v), sep) {
			out = append(out, p)
		}
		return out, nil
	})
	e.RegisterFilter("indent", func(v any, args []any) (any, error) {
		width := 4
		if len(args) > 0 {
			if n, ok := toNumber(args[0]); ok {
				width = int(n)
			}
		}
		pad := strings.Repeat(" ", width)
		lines := strings.Split(stringify(v), "\n")
		for i := 1; i < len(lines); i++ {
			if lines[i] != "" {
				lines[i] = pad + lines[i]
			}
		}
		return strings.Join(lines, "\n"), nil
	})
}
