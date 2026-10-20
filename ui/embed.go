package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:public
//go:embed all:build
var embedFS embed.FS

// Assets returns the sub-filesystem for the compiled UI assets.
func Assets() (fs.FS, error) {
	buildFS, err := fs.Sub(embedFS, "build")
	if err != nil {
		return nil, err
	}
	publicFS, err := fs.Sub(embedFS, "public")
	if err != nil {
		return nil, err
	}
	return fs.FS(compositeFS{buildFS, publicFS}), nil
}

type compositeFS []fs.FS

func (c compositeFS) Open(name string) (fs.File, error) {
	for _, fsys := range c {
		if f, err := fsys.Open(name); err == nil {
			return f, nil
		}
	}
	return nil, fs.ErrNotExist
}
