package xmlresp

import (
	"encoding/xml"
	"net/http"
)

const ContentType = "application/xml"

func Write(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", ContentType)
	w.WriteHeader(status)
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return err
	}
	return xml.NewEncoder(w).Encode(v)
}
