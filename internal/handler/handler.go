package handler

import (
	"net/http"

	"github.com/Keyhole-Koro/SynthifyShared/handlerutil"
)

func writeJSON(w http.ResponseWriter, v any) {
	handlerutil.WriteJSON(w, v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	handlerutil.WriteError(w, code, msg)
}

func decodeBody(r *http.Request, v any) error {
	return handlerutil.DecodeBody(r, v)
}
