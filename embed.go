// Package advent holds assets embedded into the server binary.
package advent

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var dist embed.FS

//go:embed data/coffees.json
var CoffeesJSON []byte

// WebFS returns the built frontend.
func WebFS() fs.FS {
	f, err := fs.Sub(dist, "web/dist")
	if err != nil {
		panic(err)
	}
	return f
}
