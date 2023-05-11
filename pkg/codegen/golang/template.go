package golang

import (
	"embed"
	"io/fs"

	"github.com/laher/mergefs"
)

//go:embed templates/*
//go:embed templates/*/*
var embeds embed.FS
var templates = mergefs.Merge(embeds)

// ParseFS adds additional filesystem's to sqlc's defaults.
//
// This allows sqlc to incorporate externally defined templates, expanding it's
// usefulness.
func ParseFS(fses ...fs.FS) {
	templates = mergefs.Merge(append(fses, embeds)...)
}
