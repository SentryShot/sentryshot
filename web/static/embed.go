package static

import (
	"embed"
)

// Static web files.
//go:embed *
var Static embed.FS
