package serve

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/samuelloranger/glim/internal/qrcode"
	"github.com/samuelloranger/glim/internal/store"
)

//go:embed ownerbar.js
var ownerBarJS string

// ownerBarData is what the bar script reads; Exp is a unix-millisecond expiry,
// 0 for a pinned preview. QR is the link as a QR code for the share panel,
// drawn by the script, so the sandboxed page needs no request to show it.
type ownerBarData struct {
	Title  string       `json:"title"`
	URL    string       `json:"url"`
	Dash   string       `json:"dash"`
	Exp    int64        `json:"exp"`
	Locked bool         `json:"locked"`
	QR     *qrcode.Code `json:"qr,omitempty"`
}

// ownerBar returns the script tag injected for the dashboard owner. The data
// goes through json.Marshal, which escapes <, > and &, so a title can never
// close the script element; the script itself is an IIFE that swallows errors.
func ownerBar(m store.Manifest, slug, base string) string {
	d := ownerBarData{
		Title:  m.Title,
		URL:    base + "/" + slug + "/",
		Dash:   base + "/",
		Locked: m.Locked(),
	}
	if d.Title == "" {
		d.Title = slug
	}
	if !m.Pinned {
		d.Exp = m.Expires.UnixMilli()
	}
	if code, err := qrcode.Encode(d.URL); err == nil {
		d.QR = &code
	}
	data, _ := json.Marshal(d)
	return "<script>(function(D){" + ownerBarJS + "})(" + string(data) + ");</script>"
}

// wantsBar reports whether r is a top-level page load, as opposed to an
// iframe or other sub-document, where a second bar would be noise.
func wantsBar(r *http.Request) bool {
	dest := r.Header.Get("Sec-Fetch-Dest")
	return dest == "" || strings.EqualFold(dest, "document")
}
