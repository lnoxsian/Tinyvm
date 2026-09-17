package web

import "embed"

// Files embeds all templates and static frontend assets.
//
//go:embed templates static vendor
var Files embed.FS
