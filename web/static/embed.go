package static

import "embed"

// Files contains public assets bundled with the server.
//
//go:embed css/*.css js/*.js fragments/*.html images/*.jpg
var Files embed.FS
