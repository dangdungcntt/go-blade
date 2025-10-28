package blade

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

var ValidFileExtensions = []string{".blade", ".tmpl", ".html", ".gohtml"}

// Engine holds loaded files.
type Engine struct {
	dirPrefix       string
	fs              fs.FS
	parsedFiles     map[string]*ParsedFile
	debugTemplates  map[string]string
	templates       map[string]*template.Template
	lastCompileTime int64
	mu              sync.Mutex
	FuncMap         template.FuncMap
}

// NewEngine creates a new engine pointing to a directory with files.
func NewEngine(dir string) *Engine {
	return NewEngineFS(os.DirFS(dir))
}

// NewEngineFS creates a new engine pointing to a filesystem.
// When using embed.Fs, pass the embedded folder as prefix.
func NewEngineFS(fs fs.FS, prefix ...string) *Engine {
	var dirPrefix string
	if len(prefix) > 0 {
		dirPrefix = prefix[0]
	}
	return &Engine{
		dirPrefix:       dirPrefix,
		fs:              fs,
		parsedFiles:     map[string]*ParsedFile{},
		debugTemplates:  map[string]string{},
		templates:       make(map[string]*template.Template),
		lastCompileTime: -1,
		FuncMap:         template.FuncMap{},
	}
}

// Load reads all files with .blade or .tmpl extension from the fs.
// It will only recompile if the files have been modified since last compile.
func (e *Engine) Load() error {
	e.mu.Lock()
	defer func() {
		e.lastCompileTime = time.Now().UnixMilli()
		e.mu.Unlock()
	}()

	needCompile := false

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

		stats, err := info.Info()
		if err != nil {
			return err
		}

		if stats.ModTime().UnixMilli() <= e.lastCompileTime {
			return nil
		}

		needCompile = true

		f, err := e.fs.Open(path)
		if err != nil {
			return err
		}
		raw, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		name := e.nameFromPath(path)
		parsedFile, err := e.parseContent(name, string(raw))
		if err != nil {
			return err
		}
		e.parsedFiles[name] = parsedFile
		return nil
	})
	if err != nil {
		return err
	}

	if !needCompile {
		return nil
	}

	// TODO: compile only changed files and dependencies

	for name, f := range e.parsedFiles {
		ctx := &CompileContext{
			Files:        e.parsedFiles,
			Yields:       map[string]YieldInfo{},
			FilledYields: map[string]struct{}{},
			Stacks:       map[string]string{},
			PushStacks:   map[string][]string{},
		}
		tmplText, err := f.ToTemplateString(ctx)
		if err != nil {
			return err
		}

		for stackName := range ctx.PushStacks {
			if _, ok := ctx.Stacks[stackName]; !ok {
				return fmt.Errorf(`[%s] missing stack "%s"`, f.Name, stackName)
			}
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
	reYield        = regexp.MustCompile(`@yield\(['"]([\w\-]+)['"](?:,\s*['"]([^)]*)['"])?\)`)   // @yield('name', 'default')
	reSectionStart = regexp.MustCompile(`@section\(['"]([\w\-]+)['"](?:,\s*['"]([^)]*)['"])?\)`) // @section('content', 'value')
	reSectionEnd   = regexp.MustCompile(`@endsection`)                                           // @endsection
	reStack        = regexp.MustCompile(`@stack\(['"]([\w\-]+)['"]\)`)                           // @stack('name')
	rePushStart    = regexp.MustCompile(`@push\(['"]([\w\-]+)['"]\)`)                            // @push('stack_name')
	rePushEnd      = regexp.MustCompile(`@endpush`)                                              // @endpush
	reInclude      = regexp.MustCompile(`@include\(['"]([\w\-/. ]+)['"](?:\s*,\s*([^)]+?))?\)`)  // @include('partial', .OtherData)
)

// parseContent parses Blade-like directives
func (e *Engine) parseContent(name string, raw string) (*ParsedFile, error) {
	p := &ParsedFile{
		Name:       name,
		Raw:        raw,
		Yields:     map[string]string{},
		Sections:   map[string]string{},
		Stacks:     map[string]struct{}{},
		PushStacks: map[string][]string{},
		ParsedAt:   time.Now().UnixMilli(),
	}
	rest := raw

	if loc := reExtend.FindStringSubmatchIndex(raw); loc != nil {
		name := rest[loc[2]:loc[3]]
		p.Extends = normalizeName(name)
		rest = rest[:loc[0]] + rest[loc[1]:]
	}

	// convert @yield to template inclusion: @yield('name') => {{ template "__yield_name" . }}
	rest = reYield.ReplaceAllStringFunc(rest, func(m string) string {
		sm := reYield.FindStringSubmatch(m)
		if len(sm) >= 3 {
			name := normalizeName(sm[1])
			p.Yields[name] = sm[2]
			return fmt.Sprintf(`{{ template "__yield_%s" . }}`, name)
		}
		return m
	})

	// convert @stack to template inclusion: @stack('name') => {{ template "__stack_name" . }}
	rest = reStack.ReplaceAllStringFunc(rest, func(m string) string {
		sm := reStack.FindStringSubmatch(m)
		if len(sm) >= 2 {
			name := normalizeName(sm[1])
			p.Stacks[name] = struct{}{}
			return fmt.Sprintf(`{{ template "__stack_%s" . }}`, name)
		}
		return m
	})

	// Parse sections
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
			return nil, fmt.Errorf("[%s] missing @endsection", p.Name)
		}
		contentStart := loc[1]
		contentEnd := loc[1] + endIdx[0]
		content := rest[contentStart:contentEnd]
		p.Sections[sectionName] = content
		// remove the section from rest by replacing with empty string
		rest = rest[:loc[0]] + rest[contentEnd+len("@endsection"):] // remove tail including @endsection
	}

	// Parse push stacks
	for {
		loc := rePushStart.FindStringSubmatchIndex(rest)
		if loc == nil {
			break
		}
		// extract section name
		stackName := rest[loc[2]:loc[3]] // matched name
		// find end
		endIdx := rePushEnd.FindStringIndex(rest[loc[1]:])
		if endIdx == nil {
			return nil, fmt.Errorf("[%s] missing @endpush", p.Name)
		}
		contentStart := loc[1]
		contentEnd := loc[1] + endIdx[0]
		content := rest[contentStart:contentEnd]
		p.PushStacks[stackName] = append(p.PushStacks[stackName], content)
		// remove the section from rest by replacing with empty string
		rest = rest[:loc[0]] + rest[contentEnd+len("@endpush"):] // remove tail including @endpush
	}

	// process includes: @include('partial') -> {{ template "__include_partial" . }}
	p.StandaloneBody = reInclude.ReplaceAllStringFunc(rest, func(m string) string {
		sm := reInclude.FindStringSubmatch(m)
		if len(sm) >= 2 {
			partialName := normalizeName(sm[1])
			pipeline := ""
			if len(sm) >= 3 {
				pipeline = strings.TrimSpace(sm[2])
			}
			if pipeline == "" {
				pipeline = "."
			}
			p.Includes = append(p.Includes, partialName)
			return fmt.Sprintf(`{{ template "__include_%s" %s }}`, partialName, pipeline)
		}
		return m
	})

	p.StandaloneBody = strings.TrimSpace(p.StandaloneBody)

	return p, nil
}

// nameFromPath converts a filesystem path to a template name, relative to engine dir.
func (e *Engine) nameFromPath(path string) string {
	rel, err := filepath.Rel(e.dirPrefix, path)
	if err != nil {
		return filepath.Base(path)
	}
	// normalize separators and drop extension
	rel = filepath.ToSlash(rel)
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	return normalizeName(rel)
}

// buildDefaultYieldContent builds default yield content for all unfilled yields.
func (e *Engine) buildDefaultYieldContent(ctx *CompileContext) string {
	var result string
	for name, info := range ctx.Yields {
		if _, ok := ctx.FilledYields[name]; !ok {
			result += `{{ define "` + name + `" }}` + info.Default + `{{ end }}`
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
