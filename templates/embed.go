package templates

import "embed"

// Files contains the pages bundled with the server.
//
//go:embed *.html
var Files embed.FS
