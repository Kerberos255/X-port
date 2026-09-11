package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var files embed.FS

func FS() (fs.FS, error) { return fs.Sub(files, "dist") }
