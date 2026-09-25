// Package plugin embeds the holler agent plugin (plugin/holler) so that
// `holler bootstrap` can install it without downloading anything. Built
// binaries under libexec/ are deliberately left out.
package plugin

import (
	"embed"
	"io/fs"
)

//go:embed all:holler/.claude-plugin all:holler/com.anthropic.claude-code holler/.mcp.json holler/skills holler/bin holler/plugin.json holler/mcp.json
var files embed.FS

// FS returns the plugin tree, rooted at the plugin directory.
func FS() fs.FS {
	sub, err := fs.Sub(files, "holler")
	if err != nil {
		panic(err)
	}
	return sub
}
