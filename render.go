package blade

import (
	"net/http"

	"github.com/gin-gonic/gin/render"
)

type View[T any] interface {
	Name() string
	Data() T
	Status() int
}

type view[T any] struct {
	name   string
	data   T
	status int
}

func NewView[T any](name string, data T, status ...int) View[T] {
	statusCode := http.StatusOK
	if len(status) > 0 {
		statusCode = status[0]
	}
	return view[T]{
		name:   name,
		data:   data,
		status: statusCode,
	}
}

func (v view[T]) Name() string {
	return v.name
}

func (v view[T]) Data() T {
	return v.data
}

func (v view[T]) Status() int {
	return v.status
}

var _ render.HTMLRender = (*HtmlRender)(nil)

// HtmlRender gin HtmlRender compatible
type HtmlRender struct {
	e *Engine
}

// NewHTMLRender create a new HtmlRender
func NewHTMLRender(e *Engine) *HtmlRender {
	return &HtmlRender{e: e}
}

// Instance returns a new render.Render
func (h *HtmlRender) Instance(name string, data any) render.Render {
	return &Render{e: h.e, name: name, data: data}
}

// Render renders HTML template with data and write to w
type Render struct {
	e    *Engine
	name string
	data any
}

// Render renders HTML template with data and writes to w
func (r *Render) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	return r.e.Render(w, r.name, r.data)
}

// WriteContentType write an HTML content type to the response header if not set
func (r *Render) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	if val := header["Content-Type"]; len(val) == 0 {
		header["Content-Type"] = []string{"text/html; charset=utf-8"}
	}
}
