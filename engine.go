package blade

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var ValidFileExtensions = []string{".blade", ".tmpl", ".html", ".gohtml"}

// Engine holds loaded files.
type Engine struct {
	fs             fs.FS
	debugTemplates map[string]string
	templates      map[string]*template.Template
	FuncMap        template.FuncMap
}

// NewEngine creates a new engine pointing to a directory with files.
func NewEngine(dir string) *Engine {
	return NewEngineFS(os.DirFS(dir))
}

// NewEngineFS creates a new engine pointing to a filesystem.
func NewEngineFS(fs fs.FS) *Engine {
	return &Engine{
		fs:             fs,
		debugTemplates: map[string]string{},
		templates:      make(map[string]*template.Template),
		FuncMap:        template.FuncMap{},
	}
}

// Load reads all files with .blade or .tmpl extension from directory (recursive).
func (e *Engine) Load() error {
	files := map[string]*ParsedFile{}
	err := fs.WalkDir(e.fs, ".", func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !slices.Contains(ValidFileExtensions, ext) {
			return nil
		}
		name := e.nameFromPath(path)
		f, err := e.fs.Open(path)
		if err != nil {
			return err
		}
		raw, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		fileContent, err := e.parseContent(string(raw))
		if err != nil {
			return err
		}
		files[name] = fileContent
		return nil
	})
	if err != nil {
		return err
	}

	for name, f := range files {
		ctx := &CompileContext{
			FilledYields: map[string]struct{}{},
			Yields:       map[string]string{},
			Files:        files,
		}
		tmplText, err := f.ToTemplateString(ctx)
		if err != nil {
			return err
		}
		tmplText += e.buildDefaultYieldContent(ctx)
		e.debugTemplates[name] = tmplText
		e.templates[name], err = template.New(name).Funcs(e.FuncMap).Parse(tmplText)
		if err != nil {
			return err
		}
	}

	return nil
}

// Render executes the template identified by entry (e.g., "pages/home") into writer with data.
func (e *Engine) Render(w io.Writer, entry string, data interface{}) error {
	entry = normalizeName(entry)
	tmpl, ok := e.templates[entry]
	if !ok {
		return fmt.Errorf("template %s not loaded", entry)
	}
	return tmpl.Execute(w, data)
}

// GetDebugTemplates returns a map of all loaded templates and their content.
func (e *Engine) GetDebugTemplates() map[string]string {
	return e.debugTemplates
}

var (
	reExtend       = regexp.MustCompile(`@extends\(['"]([\w\-/. ]+)['"]\)`)                      // allow slashes for dirs
	reSectionStart = regexp.MustCompile(`@section\(['"]([\w\-]+)['"](?:,\s*['"]([^)]*)['"])?\)`) // @section('content', 'value')
	reSectionEnd   = regexp.MustCompile(`@endsection`)                                           // @endsection
	reYield        = regexp.MustCompile(`@yield\(['"]([\w\-]+)['"](?:,\s*['"]([^)]*)['"])?\)`)   // @yield('name', 'default')
	reInclude      = regexp.MustCompile(`@include\(['"]([\w\-/. ]+)['"]\)`)
)

// parseContent parses Blade-like directives
func (e *Engine) parseContent(raw string) (*ParsedFile, error) {
	p := &ParsedFile{
		Raw:      raw,
		Sections: map[string]string{},
		Yields:   map[string]string{},
	}
	rest := raw

	if loc := reExtend.FindStringSubmatchIndex(raw); loc != nil {
		name := rest[loc[2]:loc[3]]
		p.Extends = normalizeName(name)
		rest = rest[:loc[0]] + rest[loc[1]:]
	}

	for {
		loc := reSectionStart.FindStringSubmatchIndex(rest)
		if loc == nil {
			break
		}
		// extract section name
		sectionName := rest[loc[2]:loc[3]] // matched name
		if loc[5] > -1 {
			// @section('name', 'content')
			p.Sections[sectionName] = rest[loc[4]:loc[5]]
			rest = rest[:loc[0]] + rest[loc[1]:]
			continue
		}
		// find end
		endIdx := reSectionEnd.FindStringIndex(rest[loc[1]:])
		if endIdx == nil {
			return nil, errors.New("missing @endsection")
		}
		contentStart := loc[1]
		contentEnd := loc[1] + endIdx[0]
		content := rest[contentStart:contentEnd]
		p.Sections[sectionName] = content
		// remove the section from rest by replacing with empty string
		rest = rest[:loc[0]] + rest[contentEnd+len("@endsection"):] // remove tail including @endsection
	}

	// convert @yield to template inclusion: @yield('name') => {{ template "name" . }}
	converted := reYield.ReplaceAllStringFunc(rest, func(m string) string {
		sm := reYield.FindStringSubmatch(m)
		if len(sm) >= 3 {
			name := normalizeName(sm[1])
			p.Yields[name] = sm[2]
			return fmt.Sprintf(`{{ template "%s" . }}`, name)
		}
		return m
	})

	// process includes: @include('partial') -> {{ template "partial" . }}
	p.StandaloneBody = reInclude.ReplaceAllStringFunc(converted, func(m string) string {
		sm := reInclude.FindStringSubmatch(m)
		if len(sm) >= 2 {
			name := normalizeName(sm[1])
			p.Includes = append(p.Includes, name)
			return fmt.Sprintf(`{{ template "%s" . }}`, name)
		}
		return m
	})

	p.StandaloneBody = strings.TrimSpace(p.StandaloneBody)

	return p, nil
}

// nameFromPath converts a filesystem path to a template name, relative to engine dir.
func (e *Engine) nameFromPath(path string) string {
	// normalize separators and drop extension
	rel := filepath.ToSlash(path)
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	return normalizeName(rel)
}

// buildDefaultYieldContent builds default yield content for all unfilled yields.
func (e *Engine) buildDefaultYieldContent(ctx *CompileContext) string {
	var result string
	for name, defaultValue := range ctx.Yields {
		if _, ok := ctx.FilledYields[name]; !ok {
			result += `{{ define "` + name + `" }}` + defaultValue + `{{ end }}`
		}
	}
	return result
}

// normalizeName: remove quotes/spaces and extensions, normalize slashes
func normalizeName(n string) string {
	n = strings.TrimSpace(n)
	n = strings.Trim(n, `"' `)
	// remove ext if present
	n = strings.TrimSuffix(n, filepath.Ext(n))
	n = filepath.ToSlash(n)
	return n
}
