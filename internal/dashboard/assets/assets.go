package assets

import (
	"embed"
	"net/http"
)

//go:embed styles.css logos/*
var embeddedAssets embed.FS

// FS returns an http.FileSystem for serving static assets.
func FS() http.FileSystem {
	return http.FS(embeddedAssets)
}

// ReadFile returns embedded file content by relative path.
func ReadFile(path string) ([]byte, error) {
	return embeddedAssets.ReadFile(path)
}

// LogoSVG returns the embedded logo SVG bytes for a provider name, or false if not found.
func LogoSVG(providerName string) ([]byte, bool) {
	path := "logos/" + providerName + ".svg"
	data, err := embeddedAssets.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}
