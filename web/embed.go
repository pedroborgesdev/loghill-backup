package web

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var files embed.FS

func Dist() (fs.FS, error) { return fs.Sub(files, "dist") }
