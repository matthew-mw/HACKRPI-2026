package static

import "embed"

// Files contains public assets bundled with the server.
//
//go:embed css/*.css js/*.js
var Files embed.FS
