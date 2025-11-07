package blade

import (
	"fmt"
	"html/template"
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

var _ render.HTMLRender = (*HTMLRender)(nil)

// HTMLRender gin HTMLRender compatible
type HTMLRender struct {
	e *Engine
}

// NewHTMLRender create a new HTMLRender
func NewHTMLRender(e *Engine) *HTMLRender {
	return &HTMLRender{e: e}
}

// Instance returns a new render.Render
func (h *HTMLRender) Instance(name string, data any) render.Render {
	return &Render{e: h.e, name: name, data: data}
}

type DataWithFuncs interface {
	Data() any
	Funcs() template.FuncMap
}

type dataWithFuncs struct {
	data  any
	funcs template.FuncMap
}

func NewDataWithFuncs(data any, funcs template.FuncMap) DataWithFuncs {
	return &dataWithFuncs{
		data:  data,
		funcs: funcs,
	}
}

func (d *dataWithFuncs) Data() any {
	return d.data
}

func (d *dataWithFuncs) Funcs() template.FuncMap {
	return d.funcs
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
	tmpl, ok := r.e.GetTemplate(r.name)
	if !ok {
		return fmt.Errorf("template %s not found", r.name)
	}
	if d, ok := r.data.(DataWithFuncs); ok {
		cloneTmpl, err := tmpl.Clone()
		if err != nil {
			return err
		}
		return cloneTmpl.Funcs(d.Funcs()).Execute(w, d.Data())
	}

	return tmpl.Execute(w, r.data)
}

// WriteContentType write an HTML content type to the response header if not set
func (r *Render) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	if val := header["Content-Type"]; len(val) == 0 {
		header["Content-Type"] = []string{"text/html; charset=utf-8"}
	}
}
