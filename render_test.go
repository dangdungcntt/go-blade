package blade

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewView(t *testing.T) {
	v := NewView("test", "data", 404)
	if v.Name() != "test" {
		t.Errorf("View Name mismatch, got %s", v.Name())
	}
	if v.Data() != "data" {
		t.Errorf("View Data mismatch, got %s", v.Data())
	}
	if v.Status() != 404 {
		t.Errorf("View Status mismatch, got %d", v.Status())
	}

	// Default status
	v2 := NewView("test", "data")
	if v2.Status() != http.StatusOK {
		t.Errorf("Default view status should be 200, got %d", v2.Status())
	}
}

func TestNewHTMLRender(t *testing.T) {
	engine := NewEngine("dir")
	renderer := NewHTMLRender(engine)
	if renderer == nil {
		t.Fatal("NewHTMLRender returned nil")
	}
	if renderer.e != engine {
		t.Error("Renderer engine mismatch")
	}
}

func TestRender_Instance(t *testing.T) {
	mockFS := createMockFS(map[string]string{
		"hello.blade": "Hello {{ . }}",
	})
	engine := NewEngineFS(mockFS)
	if err := engine.Load(); err != nil {
		t.Fatal(err)
	}

	renderer := NewHTMLRender(engine)
	instance := renderer.Instance("hello", "World")

	r, ok := instance.(*Render)
	if !ok {
		t.Fatal("Instance did not return *Render type")
	}
	if r.name != "hello" {
		t.Errorf("Render name mismatch: %s", r.name)
	}
	if r.data != "World" {
		t.Errorf("Render data mismatch: %v", r.data)
	}

	// Test Render Execution
	w := httptest.NewRecorder()
	err := instance.Render(w)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if w.Body.String() != "Hello World" {
		t.Errorf("Render output mismatch. Got: %s", w.Body.String())
	}
	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Content-Type mismatch. Got: %s", w.Header().Get("Content-Type"))
	}
}

func TestRender_WithFuncs(t *testing.T) {
	mockFS := createMockFS(map[string]string{
		"func.blade": "{{ upper . }}",
	})
	engine := NewEngineFS(mockFS)
	engine.FuncMap["upper"] = strings.ToUpper
	if err := engine.Load(); err != nil {
		t.Fatal(err)
	}

	renderer := NewHTMLRender(engine)

	funcs := template.FuncMap{
		"upper": strings.ToUpper,
	}
	data := NewDataWithFuncs("test", funcs)

	instance := renderer.Instance("func", data)
	w := httptest.NewRecorder()

	err := instance.Render(w)
	if err != nil {
		t.Fatalf("Refnder with funcs failed: %v", err)
	}
	if w.Body.String() != "TEST" {
		t.Errorf("Expected TEST, got %s", w.Body.String())
	}
}

func TestRender_TemplateNotFound(t *testing.T) {
	engine := NewEngineFS(createMockFS(map[string]string{}))
	renderer := NewHTMLRender(engine)
	instance := renderer.Instance("missing", nil)

	if err := instance.Render(httptest.NewRecorder()); err == nil {
		t.Error("Expected error for missing template, got nil")
	}
}

func TestDataWithFuncs(t *testing.T) {
	funcs := template.FuncMap{}
	d := NewDataWithFuncs("data", funcs)

	if d.Data() != "data" {
		t.Error("Data getter mismatch")
	}

	// Check func map equality (pointers/len)
	if len(d.Funcs()) != 0 { // comparing maps is hard, just len check here
		t.Error("Funcs mismatch")
	}
}
