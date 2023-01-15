//go:build static
// +build static

package static

import (
	"embed"
	"net/http"
)

//go:embed *
var content embed.FS

func ServeHTML(mux *http.ServeMux) {
	http.HandleFunc("/", http.FileServer(http.FS(content)))
}
