package blade

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestNewEngine(t *testing.T) {
	// Test NewEngine with directory
	t.Run("NewEngine_Dir", func(t *testing.T) {
		dir := "examples/views"
		engine := NewEngine(dir)
		if engine == nil {
			t.Fatal("NewEngine returned nil")
		}
		if engine.fs == nil {
			t.Error("engine.fs should not be nil")
		}
	})

	// Test NewEngineFS with mock FS
	t.Run("NewEngineFS_Mock", func(t *testing.T) {
		mockFS := createMockFS(map[string]string{
			"test.blade": "hello",
		})
		engine := NewEngineFS(mockFS)
		if engine == nil {
			t.Fatal("NewEngineFS returned nil")
		}
		if engine.fs == nil {
			t.Error("engine.fs should not be nil")
		}
	})
}

func TestLoadAndRender(t *testing.T) {
	mockFS := createMockFS(map[string]string{
		"pages/home.blade":       `@extends("layouts/base") @section("content") <h1>Home</h1> @endsection`,
		"layouts/base.blade":     `<html><body>@yield("content") <footer>@include("partials.footer")</footer></body></html>`,
		"partials/footer.blade":  `Copyright 2024`,
		"_ignored.blade":         `should not be loaded`,
		"partials/_hidden.blade": `should not be loaded`,
	})

	engine := NewEngineFS(mockFS)

	// Test Load
	err := engine.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Test Templates Existence
	expectedTemplates := []string{"pages/home", "layouts/base", "partials/footer"}
	for _, name := range expectedTemplates {
		if _, ok := engine.GetTemplate(name); !ok {
			t.Errorf("Template %s not found", name)
		}
	}

	// Test Filtered Templates
	filteredTemplates := []string{"_ignored", "partials/_hidden"}
	for _, name := range filteredTemplates {
		if _, ok := engine.GetTemplate(name); ok {
			t.Errorf("Template %s should have been filtered out", name)
		}
	}

	// Test Render
	var buf bytes.Buffer
	err = engine.Render(&buf, "pages/home", nil)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expectedOutput := `<html><body><h1>Home</h1> <footer>Copyright 2024</footer></body></html>`
	if normalizeSpace(buf.String()) != normalizeSpace(expectedOutput) {
		t.Errorf("Render output mismatch.\nExpected: %s\nGot: %s", expectedOutput, buf.String())
	}
}

func TestRender_Errors(t *testing.T) {
	mockFS := createMockFS(map[string]string{
		"valid.blade": "Valid",
	})
	engine := NewEngineFS(mockFS)
	_ = engine.Load()

	t.Run("TemplateNotFound", func(t *testing.T) {
		err := engine.Render(os.Stdout, "nonexistent", nil)
		if err == nil {
			t.Error("Is expected error for nonexistent template, got nil")
		}
	})
}

func TestParseFile_Directives(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectedBody  string // simplified check
		expectedError string
	}{
		{
			name:         "Simple Content",
			content:      "Hello World",
			expectedBody: "Hello World",
		},
		{
			name:         "Yield",
			content:      `@yield("content", "default")`,
			expectedBody: `{{ template "__section_content" . }}`,
		},
		{
			name:         "Include",
			content:      `@include("partials.header")`,
			expectedBody: `{{ template "__partial_partials/header" . }}`,
		},
		{
			name:         "Include with data",
			content:      `@include("partials.alert", "some data")`,
			expectedBody: `{{ template "__partial_partials/alert" "some data" }}`,
		},
		{
			name:    "Stack and Push",
			content: `@push("scripts") <script>alert(1)</script> @endpush @stack("scripts")`,
			// Note: The @push part is removed from body and stored in PushStacks, @stack becomes a template call
			expectedBody: `{{ template "__stack_scripts" . }}`,
		},
		{
			name:          "Unclosed Section",
			content:       `@section("main") content`,
			expectedError: "missing @endsection",
		},
		{
			name:          "Unclosed Push",
			content:       `@push("scripts") var x=1;`,
			expectedError: "missing @endpush",
		},
	}

	engine := NewEngineFS(fstest.MapFS{})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := engine.parseFile("test", tc.content)
			if tc.expectedError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("expected error containing %q, got %v", tc.expectedError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(parsed.StandaloneBody, tc.expectedBody) {
					t.Errorf("expected body to contain %q, got %q", tc.expectedBody, parsed.StandaloneBody)
				}
			}
		})
	}
}

func TestFuncMap(t *testing.T) {
	mockFS := createMockFS(map[string]string{
		"greet.blade": `{{ upper "hello" }}`,
	})
	engine := NewEngineFS(mockFS)
	engine.FuncMap["upper"] = strings.ToUpper
	if err := engine.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	var buf bytes.Buffer
	if err := engine.Render(&buf, "greet", nil); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if buf.String() != "HELLO" {
		t.Errorf("Expected HELLO, got %q", buf.String())
	}
}

func TestCustomEntryFilter(t *testing.T) {
	mockFS := createMockFS(map[string]string{
		"admin.blade":  "admin",
		"public.blade": "public",
	})
	engine := NewEngineFS(mockFS)
	engine.EntryFilter = func(f *ParsedFile) bool {
		return f.Name == "public"
	}

	if err := engine.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if _, ok := engine.GetTemplate("admin"); ok {
		t.Error("admin should be filtered out")
	}
	if _, ok := engine.GetTemplate("public"); !ok {
		t.Error("public should be loaded")
	}
}

func TestGetDebugTemplates(t *testing.T) {
	mockFS := createMockFS(map[string]string{
		"test.blade": "content",
	})
	engine := NewEngineFS(mockFS)
	_ = engine.Load()

	debug := engine.GetDebugTemplates()
	if len(debug) == 0 {
		t.Error("Expected debug templates, got empty map")
	}
	if _, ok := debug["test"]; !ok {
		t.Error("Expected 'test' in debug templates")
	}
}

func TestValidation_PushStack(t *testing.T) {
	// Case: Push to undefined stack
	mockFS := createMockFS(map[string]string{
		"bad_push.blade": `@push("unknown") val @endpush`,
	})
	engine := NewEngineFS(mockFS)
	engine.IgnoreInvalidPushStack = false

	if err := engine.Load(); err == nil {
		t.Error("Expected error when pushing to unknown stack, got nil")
	}

	// Case: Ignore invalid push stack
	engine.IgnoreInvalidPushStack = true
	if err := engine.Load(); err != nil {
		t.Errorf("Expected no error when IgnoreInvalidPushStack is true, got: %v", err)
	}
}

func TestComplexInheritance(t *testing.T) {
	mockFS := createMockFS(map[string]string{
		"master.blade": `M_Start @yield("l1") M_End`,
		"l1.blade":     `@extends("master") @section("l1") L1_Start @yield("l2") L1_End @endsection`,
		"l2.blade":     `@extends("l1") @section("l2") Content @endsection`,
	})
	engine := NewEngineFS(mockFS)
	if err := engine.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	var buf bytes.Buffer
	if err := engine.Render(&buf, "l2", nil); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "M_Start L1_Start Content L1_End M_End"
	if normalizeSpace(buf.String()) != normalizeSpace(expected) {
		t.Errorf("Complex inheritance mismatch.\nExp: %s\nGot: %s", expected, normalizeSpace(buf.String()))
	}
}

func TestNormalizeName(t *testing.T) {
	// Since normalizeName is unexported, we can test it via nameFromPath or indirectly
	// For package-level tests, we can access unexported symbols if in same package.
	tests := []struct {
		input    string
		expected string
	}{
		{"  path/to/file  ", "path/to/file"},
		{"path.to.file", "path/to/file"},
		{`"quoted"`, "quoted"},
	}

	for _, tc := range tests {
		got := normalizeName(tc.input) // allowed as we are in package blade
		if got != tc.expected {
			t.Errorf("normalizeName(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func createMockFS(files map[string]string) fstest.MapFS {
	fs := fstest.MapFS{}
	now := time.Now()
	for name, content := range files {
		fs[name] = &fstest.MapFile{
			Data:    []byte(content),
			ModTime: now,
		}
	}
	return fs
}

// Helpers

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
