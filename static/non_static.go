//go:build !static
// +build !static

package static

import "net/http"

func ServeHTML() {
	http.Handle("/", http.FileServer(http.Dir("static/")))
}
