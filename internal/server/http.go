package server

import (
	"errors"
	"net/http"

	"github.com/gloss-mcp/client/internal/store"
)

// returnTo resolves where a mutation should send the browser back to: the
// form's explicit return_to field if present, otherwise the Referer
// header, otherwise the repo root.
func returnTo(r *http.Request) string {
	if v := r.FormValue("return_to"); v != "" {
		return v
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		return ref
	}
	return "/"
}

// redirectBack sends the browser back to url after a mutation: an
// HX-Redirect for HTMX requests (which otherwise expect a swappable
// response body to target), a normal 303 redirect for plain form
// submissions.
func redirectBack(w http.ResponseWriter, r *http.Request, url string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// writeStoreError maps a store error to an HTTP response.
func writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}
