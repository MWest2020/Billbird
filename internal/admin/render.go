package admin

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
)

// layoutData is what every admin page passes to the shared "layout.html"
// template. Content is the already-rendered HTML for the page's main body,
// produced by renderTemplate.
type layoutData struct {
	Title    string
	Active   string
	Username string
	Content  template.HTML
}

func (h *Handler) renderLayout(w http.ResponseWriter, data layoutData) {
	w.Header().Set("Content-Type", "text/html")
	if err := h.templates.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("error rendering layout: %v", err)
	}
}

func (h *Handler) renderTemplate(name string, data any) template.HTML {
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("error rendering %s: %v", name, err)
		return template.HTML("<p>Error rendering template</p>")
	}
	return template.HTML(buf.String())
}
