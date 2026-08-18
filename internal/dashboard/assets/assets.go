package assets

import (
	"embed"
	"net/http"
)

//go:embed styles.css logos/* *.js
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
// It tries multiple naming conventions: exact match, -dark suffix, -mono suffix.
func LogoSVG(providerName string) ([]byte, bool) {
	// Try exact match first
	path := "logos/" + providerName + ".svg"
	data, err := embeddedAssets.ReadFile(path)
	if err == nil {
		return data, true
	}

	// Try dark variant
	darkPath := "logos/" + providerName + "-dark.svg"
	data, err = embeddedAssets.ReadFile(darkPath)
	if err == nil {
		return data, true
	}

	// Try mono variant
	monoPath := "logos/" + providerName + "-mono.svg"
	data, err = embeddedAssets.ReadFile(monoPath)
	if err == nil {
		return data, true
	}

	return nil, false
}
