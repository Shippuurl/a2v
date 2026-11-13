package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Brand palette: graphite ink on the terminal's own background, with an
// ice-teal → indigo → violet sweep for anything that moves.
var (
	ink    = lipgloss.Color("#e6edf3")
	subtle = lipgloss.Color("#8b949e")
	faint  = lipgloss.Color("#4b5563")
	accent = lipgloss.Color("#5eead4")
	iris   = lipgloss.Color("#818cf8")
	orchid = lipgloss.Color("#c084fc")
	good   = lipgloss.Color("#4ade80")
	bad    = lipgloss.Color("#fb7185")
	warn   = lipgloss.Color("#fbbf24")
)

func style(c lipgloss.TerminalColor) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

type rgb struct{ r, g, b float64 }

func parseHex(s string) rgb {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return rgb{}
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgb{}
	}
	return rgb{float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255}
}

func (c rgb) hex() string {
	f := func(v float64) int { return int(math.Round(math.Max(0, math.Min(1, v)) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", f(c.r), f(c.g), f(c.b))
}

func (c rgb) mix(o rgb, t float64) rgb {
	return rgb{c.r + (o.r-c.r)*t, c.g + (o.g-c.g)*t, c.b + (o.b-c.b)*t}
}

// stops for the brand sweep.
var sweep = []rgb{parseHex("#5eead4"), parseHex("#818cf8"), parseHex("#c084fc")}

// sample returns the gradient colour at position t∈[0,1).
func sample(t float64) rgb {
	if t < 0 {
		t -= math.Floor(t)
	}
	t = t - math.Floor(t)
	t *= float64(len(sweep) - 1)
	i := int(t)
	if i >= len(sweep)-1 {
		return sweep[len(sweep)-1]
	}
	return sweep[i].mix(sweep[i+1], t-float64(i))
}

// gradient paints each rune along the brand sweep, offset by phase so the
// colour visibly travels when phase advances.
func gradient(s string, phase float64) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	var b strings.Builder
	for i, ch := range r {
		if ch == ' ' {
			b.WriteRune(ch)
			continue
		}
		t := float64(i)/float64(len(r))*0.75 + phase
		b.WriteString(style(lipgloss.Color(sample(t).hex())).Render(string(ch)))
	}
	return b.String()
}

// shimmer draws a divider with a bright pulse travelling across it.
func shimmer(width int, phase float64, base lipgloss.Color) string {
	if width <= 0 {
		return ""
	}
	baseRGB := parseHex(string(base))
	head := phase * float64(width)
	var b strings.Builder
	for i := 0; i < width; i++ {
		d := math.Abs(float64(i) - head)
		if d > float64(width)/2 {
			d = float64(width) - d
		}
		glow := math.Exp(-d * d / 26.0)
		if glow < 0.2 {
			b.WriteString(style(base).Render("─"))
			continue
		}
		target := sample(0.15 + 0.5*float64(i)/float64(width))
		b.WriteString(style(lipgloss.Color(baseRGB.mix(target, math.Min(1, glow)).hex())).Render("─"))
	}
	return b.String()
}

// trunc cuts a string to w display cells, adding an ellipsis.
func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	out := make([]rune, 0, len(r))
	cur := 0
	for i, ch := range r {
		cw := lipgloss.Width(string(ch))
		if cur+cw > w-1 {
			_ = i
			break
		}
		out = append(out, ch)
		cur += cw
	}
	return string(out) + "…"
}

// fill pads s with spaces to exactly w cells (display width aware).
func fill(s string, w int) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return trunc(s, w)
	}
	return s + strings.Repeat(" ", gap)
}

// hint renders "key label" chips for the footer.
func hint(key, label string) string {
	k := style(faint).Render(" ")
	if key != "" {
		k = style(lipgloss.Color("#9ca3af")).Bold(true).Render(key)
	}
	return k + style(faint).Render(" "+label+" ")
}

// logo renders the wordmark plus tagline. The triangle is the brand mark
// from the A2VEC identity; its colour walks the sweep over time.
func logo(phase float64, tagline string) string {
	mark := gradient("▲", phase)
	word := lipgloss.NewStyle().Bold(true).Foreground(ink).Render("a2v")
	tag := style(faint).Render(tagline)
	return "  " + mark + "  " + word + "  " + tag
}

// pill renders a rounded label used for proxy / model / dir chips.
func pill(label, value string, c lipgloss.TerminalColor) string {
	l := style(faint).Render(label + " ")
	v := lipgloss.NewStyle().Foreground(c).Render(value)
	return l + v
}

// badge renders the alias tag shown at the right of a model row.
func badge(s string) string {
	if s == "" {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0b1220")).
		Background(lipgloss.Color(sample(0.45).hex())).
		Padding(0, 1).
		Render(s)
}
