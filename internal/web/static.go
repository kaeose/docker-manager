package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

func GetStaticFS() http.FileSystem {
	staticFS, _ := fs.Sub(staticFiles, "static")
	return http.FS(staticFS)
}

func ReadIndex() ([]byte, error) {
	return staticFiles.ReadFile("static/index.html")
}
