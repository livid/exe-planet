package build

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// A port of Planet/TemplateBrowser/ColorFilter.swift (itself from
// codepen.io/sosuke/pen/Pjoqqp): finds a CSS filter chain that turns a
// black glyph into a target colour. The search is stochastic in Planet
// too, so two runs give different but equally close chains; a build
// reuses Planet's cached values when it has them.

type rgb struct{ r, g, b float64 }

func clamp(v float64) float64 { return math.Min(math.Max(v, 0), 255) }

func (c *rgb) set(r, g, b float64) { c.r, c.g, c.b = clamp(r), clamp(g), clamp(b) }

func (c *rgb) multiply(m [9]float64) {
	r := clamp(c.r*m[0] + c.g*m[1] + c.b*m[2])
	g := clamp(c.r*m[3] + c.g*m[4] + c.b*m[5])
	b := clamp(c.r*m[6] + c.g*m[7] + c.b*m[8])
	c.r, c.g, c.b = r, g, b
}

func (c *rgb) hueRotate(angle float64) {
	rad := angle / 180 * math.Pi
	s, co := math.Sin(rad), math.Cos(rad)
	c.multiply([9]float64{
		0.213 + co*0.787 - s*0.213, 0.715 - co*0.715 - s*0.715, 0.072 - co*0.072 + s*0.928,
		0.213 - co*0.213 + s*0.143, 0.715 + co*0.285 + s*0.140, 0.072 - co*0.072 - s*0.283,
		0.213 - co*0.213 - s*0.787, 0.715 - co*0.715 + s*0.715, 0.072 + co*0.928 + s*0.072,
	})
}

func (c *rgb) sepia(v float64) {
	c.multiply([9]float64{
		0.393 + 0.607*(1-v), 0.769 - 0.769*(1-v), 0.189 - 0.189*(1-v),
		0.349 - 0.349*(1-v), 0.686 + 0.314*(1-v), 0.168 - 0.168*(1-v),
		0.272 - 0.272*(1-v), 0.534 - 0.534*(1-v), 0.131 + 0.869*(1-v),
	})
}

func (c *rgb) saturate(v float64) {
	c.multiply([9]float64{
		0.213 + 0.787*v, 0.715 - 0.715*v, 0.072 - 0.072*v,
		0.213 - 0.213*v, 0.715 + 0.285*v, 0.072 - 0.072*v,
		0.213 - 0.213*v, 0.715 - 0.715*v, 0.072 + 0.928*v,
	})
}

func (c *rgb) linear(slope, intercept float64) {
	c.r = clamp(c.r*slope + intercept*255)
	c.g = clamp(c.g*slope + intercept*255)
	c.b = clamp(c.b*slope + intercept*255)
}

func (c *rgb) brightness(v float64) { c.linear(v, 0) }
func (c *rgb) contrast(v float64)   { c.linear(v, -(0.5*v)+0.5) }

func (c *rgb) invert(v float64) {
	c.r = clamp((v + c.r/255*(1-2*v)) * 255)
	c.g = clamp((v + c.g/255*(1-2*v)) * 255)
	c.b = clamp((v + c.b/255*(1-2*v)) * 255)
}

func (c *rgb) hsl() (h, s, l float64) {
	r, g, b := c.r/255, c.g/255, c.b/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l * 100
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	case b:
		h = (r-g)/d + 4
	}
	h /= 6
	return h * 100, s * 100, l * 100
}

// SupportsHexFilter reports whether a setting value is a six-digit hex
// colour, with or without the #.
func SupportsHexFilter(v string) bool {
	s := strings.TrimPrefix(strings.TrimSpace(v), "#")
	if len(s) != 6 {
		return false
	}
	_, err := strconv.ParseUint(s, 16, 32)
	return err == nil
}

func parseHex(v string) rgb {
	s := strings.TrimFunc(v, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	})
	if len(s) != 6 {
		return rgb{}
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgb{}
	}
	return rgb{float64(n >> 16 & 0xff), float64(n >> 8 & 0xff), float64(n & 0xff)}
}

type solver struct {
	target     rgb
	th, ts, tl float64
	rng        *rand.Rand
}

// SolveColorFilter returns the CSS filter chain for a hex colour.
func SolveColorFilter(hex string) string {
	return solveColorFilter(hex, rand.New(rand.NewSource(rand.Int63())))
}

func solveColorFilter(hex string, rng *rand.Rand) string {
	s := &solver{target: parseHex(hex), rng: rng}
	s.th, s.ts, s.tl = s.target.hsl()
	wide, wideLoss := s.solveWide()
	values, _ := s.spsa(wideLoss, [6]float64{0.25 * (wideLoss + 1), 0.25 * (wideLoss + 1), wideLoss + 1, 0.25 * (wideLoss + 1), 0.2 * (wideLoss + 1), 0.2 * (wideLoss + 1)}, 2, wide, 500)
	return css(values)
}

func (s *solver) solveWide() ([6]float64, float64) {
	best, bestLoss := [6]float64{}, math.Inf(1)
	for i := 0; i < 3 && bestLoss > 25; i++ {
		v, l := s.spsa(5, [6]float64{60, 180, 18000, 600, 1.2, 1.2}, 15, [6]float64{50, 20, 3750, 50, 100, 100}, 1000)
		if l < bestLoss {
			best, bestLoss = v, l
		}
	}
	return best, bestLoss
}

func (s *solver) spsa(A float64, a [6]float64, c float64, values [6]float64, iters int) ([6]float64, float64) {
	const alpha, gamma = 1.0, 0.16666666666666666
	var best [6]float64
	bestLoss := math.Inf(1)
	var deltas, high, low [6]float64
	for k := 0; k < iters; k++ {
		ck := c / math.Pow(float64(k+1), gamma)
		for i := 0; i < 6; i++ {
			if s.rng.Intn(2) == 0 {
				deltas[i] = 1
			} else {
				deltas[i] = -1
			}
			high[i] = values[i] + ck*deltas[i]
			low[i] = values[i] - ck*deltas[i]
		}
		lossDiff := s.loss(high) - s.loss(low)
		for i := 0; i < 6; i++ {
			g := lossDiff / (2 * ck) * deltas[i]
			ak := a[i] / math.Pow(A+float64(k+1), alpha)
			values[i] = fix(values[i]-ak*g, i)
		}
		if l := s.loss(values); l < bestLoss {
			best, bestLoss = values, l
		}
	}
	if math.IsInf(bestLoss, 1) {
		return values, bestLoss
	}
	return best, bestLoss
}

func fix(v float64, idx int) float64 {
	max := 100.0
	if idx == 2 {
		max = 7500
	} else if idx == 4 || idx == 5 {
		max = 200
	}
	if idx == 3 {
		if v > max {
			return math.Mod(v, max)
		} else if v < 0 {
			return max + math.Mod(v, max)
		}
	} else if v < 0 {
		return 0
	} else if v > max {
		return max
	}
	return v
}

func (s *solver) loss(f [6]float64) float64 {
	c := rgb{}
	c.invert(f[0] / 100)
	c.sepia(f[1] / 100)
	c.saturate(f[2] / 100)
	c.hueRotate(f[3] * 3.6)
	c.brightness(f[4] / 100)
	c.contrast(f[5] / 100)
	h, sat, l := c.hsl()
	return math.Abs(c.r-s.target.r) + math.Abs(c.g-s.target.g) + math.Abs(c.b-s.target.b) +
		math.Abs(h-s.th) + math.Abs(sat-s.ts) + math.Abs(l-s.tl)
}

func css(f [6]float64) string {
	r := func(v float64) string { return strconv.FormatFloat(math.Round(v), 'f', 1, 64) }
	return fmt.Sprintf("invert(%s%%) sepia(%s%%) saturate(%s%%) hue-rotate(%sdeg) brightness(%s%%) contrast(%s%%)",
		r(f[0]), r(f[1]), r(f[2]), r(f[3]*3.6), r(f[4]), r(f[5]))
}

// SettingsWithFilters is Planet's buildTemplateSettingsAndFilters: every
// setting whose key ends in Color and holds a hex colour gets a
// <key>Filter, from the cache when present, else solved now.
func SettingsWithFilters(settings, cache map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range settings {
		out[k] = v
	}
	var keys []string
	for k, v := range settings {
		if strings.HasSuffix(k, "Color") && SupportsHexFilter(v) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if cached, ok := cache[k+"Filter"]; ok {
			out[k+"Filter"] = cached
			continue
		}
		out[k+"Filter"] = SolveColorFilter(settings[k])
	}
	return out
}
