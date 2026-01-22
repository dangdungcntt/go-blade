package blade

import (
	"testing"
)

func TestDefaultEntryFilter(t *testing.T) {
	engine := NewEngine("examples/views")
	engine.FuncMap["hello"] = func(name string) string {
		return "Hello " + name
	}
	if err := engine.Load(); err != nil {
		t.Fatalf("failed to load engine: %v", err)
	}

	visibleFiles := []string{
		"pages/home",
		"pages/about",
		"layouts/base",
		"layouts/home",
	}

	filteredFiles := []string{
		"_partials/nav",
		"_partials/menu-items",
		"_partials/profile",
		"_ignored",
	}

	// Verify visible files are present as templates
	for _, name := range visibleFiles {
		if _, ok := engine.GetTemplate(name); !ok {
			t.Errorf("expected template %q to be present, but it was not found", name)
		}
	}

	for _, name := range filteredFiles {
		// Note: normalization might be needed if not handled by GetTemplate for these checks,
		// but GetTemplate calls normalizeName internally.
		if _, ok := engine.GetTemplate(name); ok {
			t.Errorf("expected template %q to be filtered out, but it was found", name)
		}
	}
}
