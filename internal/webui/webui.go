package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets
var webAssets embed.FS

// Handler returns an HTTP handler for the GraphJin Web UI console
// routePrefix is the URL path prefix where the WebUI will be mounted (e.g., "/console")
// gqlEndpoint is the GraphQL endpoint URL that the WebUI will communicate with (e.g., "/graphql")
func Handler(routePrefix string, gqlEndpoint string) http.Handler {
	// Get the embedded assets subdirectory
	webRoot, err := fs.Sub(webAssets, "assets")
	if err != nil {
		// If we can't get the assets, return a handler that shows an error
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "WebUI assets not available", http.StatusInternalServerError)
		})
	}

	// Create file server for the embedded assets
	fileServer := http.FileServer(http.FS(webRoot))

	// Handler function that serves the UI
	handler := func(w http.ResponseWriter, r *http.Request) {
		// If accessing the root with no query params, redirect to include the GraphQL endpoint
		if r.URL.Path == "" && r.URL.RawQuery == "" {
			redirectURL := r.URL.Path + "?endpoint=" + gqlEndpoint
			w.Header().Set("Location", redirectURL)
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}

		// Serve the file
		fileServer.ServeHTTP(w, r)
	}

	// Ensure routePrefix ends with /
	if !strings.HasSuffix(routePrefix, "/") {
		routePrefix += "/"
	}

	// Strip the prefix and serve
	return http.StripPrefix(routePrefix, http.HandlerFunc(handler))
}
