// Package bladish — a small Blade-like templating preprocessor for Go
// Features:
//  - @extends('base') to inherit a layout
//  - @section('name') ... @endsection to define sections
//  - @yield('name') to place sections in layout (supports default content)
//  - @include('partial') to include other files
// Usage: see example at the bottom (main function)

package blade

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var validFileExts = []string{".blade", ".tmpl", ".html"}

// Engine holds loaded files.
type Engine struct {
	dir            string
	debugTemplates map[string]string
	templates      map[string]*template.Template
}

// New creates a new engine pointing to a directory with files.
func New(dir string) *Engine {
	return &Engine{
		dir:            dir,
		debugTemplates: map[string]string{},
		templates:      make(map[string]*template.Template),
	}
}

// Load reads all files with .blade or .tmpl extension from directory (recursive).
func (e *Engine) Load() error {
	files := map[string]*ParsedFile{}
	err := filepath.Walk(e.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !slices.Contains(validFileExts, ext) {
			return nil
		}
		name := e.nameFromPath(path)
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := e.parseContent(string(raw))
		if err != nil {
			return err
		}
		files[name] = f
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
		e.templates[name], err = template.New(name).Parse(tmplText)
		if err != nil {
			return err
		}
	}

	return nil
}

// nameFromPath converts filesystem path to template name, relative to engine dir.
func (e *Engine) nameFromPath(path string) string {
	rel, err := filepath.Rel(e.dir, path)
	if err != nil {
		return filepath.Base(path)
	}
	// normalize separators and drop extension
	rel = filepath.ToSlash(rel)
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	return normalizeName(rel)
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
		// section content between start end
		contentStart := loc[1]
		contentEnd := loc[1] + endIdx[0]
		content := rest[contentStart:contentEnd]
		p.Sections[sectionName] = content
		// remove the section from rest by replacing with empty string
		rest = rest[:loc[0]] + rest[contentEnd+len("@endsection"):] // remove tail including @endsection
	}

	// no extends — this is a base (layout) or standalone file
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

func (e *Engine) buildDefaultYieldContent(ctx *CompileContext) string {
	var result string
	for name, defaultValue := range ctx.Yields {
		if _, ok := ctx.FilledYields[name]; !ok {
			result += `{{ define "` + name + `" }}` + defaultValue + `{{ end }}`
		}
	}
	return result
}

// Render executes the template identified by entry (e.g., "pages/home") into writer with data.
func (e *Engine) Render(w io.Writer, entry string, data interface{}) error {
	entry = normalizeName(entry)
	tmpl, ok := e.templates[entry]
	if !ok {
		return fmt.Errorf("template %s not loaded", entry)
	}
	return tmpl.ExecuteTemplate(w, entry, data)
}

// GetDebugTemplates returns a map of all loaded templates and their content.
func (e *Engine) GetDebugTemplates() map[string]string {
	return e.debugTemplates
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
