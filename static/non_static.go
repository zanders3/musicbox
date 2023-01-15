//go:build !static
// +build !static

package static

import "net/http"

func ServeHTML(mux *http.ServeMux) {
	mux.Handle("/", http.FileServer(http.Dir("static/")))
}
