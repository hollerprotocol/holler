package tui

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
)

// markdown renders message text with glamour, sized for chat bubbles:
// no document margin, cached per message, width and background.
type markdown struct {
	dark  bool
	width int
	r     *glamour.TermRenderer
	cache map[string]string
}

func (md *markdown) render(key, text string, width int, dark bool) string {
	if md.r == nil || md.width != width || md.dark != dark {
		cfg := styles.LightStyleConfig
		if dark {
			cfg = styles.DarkStyleConfig
		}
		zero := uint(0)
		empty := ""
		cfg.Document.Margin = &zero
		cfg.Document.BlockPrefix = empty
		cfg.Document.BlockSuffix = empty
		cfg.CodeBlock.Margin = &zero
		// Keep single newlines: agents paste command output and lists
		// without blank lines, and reflowing them into one paragraph
		// garbles them.
		r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(width), glamour.WithPreservedNewLines())
		if err != nil {
			return text
		}
		md.r, md.width, md.dark, md.cache = r, width, dark, map[string]string{}
	}
	ck := fmt.Sprintf("%s\x00%d", key, width)
	if s, ok := md.cache[ck]; ok {
		return s
	}
	out, err := md.r.Render(text)
	if err != nil {
		out = text
	}
	out = strings.Trim(out, "\n")
	// glamour pads lines with spaces to the wrap width; drop them so
	// bubbles hug their content.
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	out = strings.Join(lines, "\n")
	md.cache[ck] = out
	return out
}
