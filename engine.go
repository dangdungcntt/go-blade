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

var DefaultValidFileExtensions = []string{".blade", ".tmpl", ".html", ".gohtml"}

// EntryFilter is a function that determines whether a parsed file should be available as a view
type EntryFilter func(file *ParsedFile) bool

// DefaultEntryFilter excludes files that start with an underscore or are in a directory that starts with an underscore
var DefaultEntryFilter = func(file *ParsedFile) bool {
	return !strings.HasPrefix(file.Name, "_") && !strings.Contains(file.Name, "/_")
}

// Engine holds loaded files.
type Engine struct {
	dirPrefix              string
	fs                     fs.FS
	parsedFiles            map[string]*ParsedFile
	debugTemplates         map[string]string
	templates              map[string]*template.Template
	lastCompileTime        int64
	mu                     sync.Mutex
	ValidFileExtensions    []string
	FuncMap                template.FuncMap
	EntryFilter            EntryFilter
	IgnoreInvalidPushStack bool
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

	validExts := make([]string, len(DefaultValidFileExtensions))
	copy(validExts, DefaultValidFileExtensions)

	return &Engine{
		dirPrefix:              dirPrefix,
		fs:                     fs,
		parsedFiles:            map[string]*ParsedFile{},
		debugTemplates:         map[string]string{},
		templates:              make(map[string]*template.Template),
		lastCompileTime:        -1,
		ValidFileExtensions:    validExts,
		FuncMap:                template.FuncMap{},
		EntryFilter:            DefaultEntryFilter,
		IgnoreInvalidPushStack: false,
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
		if !slices.Contains(e.ValidFileExtensions, ext) {
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
		parsedFile, err := e.parseFile(name, string(raw))
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
		if !e.EntryFilter(f) {
			continue
		}
		ctx := &CompileContext{
			Files:          e.parsedFiles,
			Yields:         map[string]YieldInfo{},
			FilledSections: map[string]struct{}{},
			FilledIncludes: map[string]struct{}{},
			Stacks:         map[string]string{},
			PushStacks:     map[string][]string{},
		}
		bodyText, defText, err := f.ToTemplateString(ctx)
		if err != nil {
			return err
		}

		if !e.IgnoreInvalidPushStack {
			for stackName := range ctx.PushStacks {
				if _, ok := ctx.Stacks[stackName]; !ok {
					return fmt.Errorf(`[%s] missing stack "%s"`, f.Name, stackName)
				}
			}
		}

		defText += e.buildDefaultYieldContent(ctx)
		tmplText := defText + bodyText
		e.debugTemplates[name] = tmplText
		e.templates[name], err = template.New(name).Funcs(e.FuncMap).Parse(tmplText)
		if err != nil {
			// TODO: parse template error to point to the debug template content
			return err
		}
	}

	return nil
}

// Render executes the template identified by entry (e.g., "pages/home") into io.Writer with data.
func (e *Engine) Render(w io.Writer, entry string, data any) error {
	tmpl, ok := e.GetTemplate(entry)
	if !ok {
		return fmt.Errorf("template %s not loaded", entry)
	}
	return tmpl.Execute(w, data)
}

// GetTemplate returns the template identified by entry.
func (e *Engine) GetTemplate(entry string) (*template.Template, bool) {
	entry = normalizeName(entry)
	tmpl, ok := e.templates[entry]
	return tmpl, ok
}

// GetDebugTemplates returns a map of all loaded templates and their content.
func (e *Engine) GetDebugTemplates() map[string]string {
	return e.debugTemplates
}

var (
	reExtend     = regexp.MustCompile(`@extends\(['"]([\w\-/. ]+)['"]\)`)                    // allow slashes for dirs
	reYield      = regexp.MustCompile(`@yield\(['"]([\w\-]+)['"](?:,\s*['"]([^)]*)['"])?\)`) //	@yield('name',	'default')
	reSectionEnd = regexp.MustCompile(`@endsection`)                                         //	@endsection
	reStack      = regexp.MustCompile(`@stack\(['"]([\w\-]+)['"]\)`)                         //	@stack('name')
	rePushStart  = regexp.MustCompile(`@push\(['"]([\w\-]+)['"]\)`)                          //	@push('stack_name')
	rePushEnd    = regexp.MustCompile(`@endpush`)                                            //	@endpush
)

// parseFile parses Blade-like directives
func (e *Engine) parseFile(name string, raw string) (*ParsedFile, error) {
	p := &ParsedFile{
		Name:       name,
		Raw:        raw,
		Includes:   map[string]struct{}{},
		Yields:     map[string]string{},
		Sections:   map[string]string{},
		Stacks:     map[string]struct{}{},
		PushStacks: map[string][]string{},
		ParsedAt:   time.Now().UnixMilli(),
	}
	rest := raw

	if loc := reExtend.FindStringSubmatchIndex(raw); loc != nil {
		parentName := rest[loc[2]:loc[3]]
		p.Extends = normalizeName(parentName)
		rest = rest[:loc[0]] + rest[loc[1]:]
	}

	// convert @yield to template inclusion: @yield('name') => {{ template "__section_name" . }}
	rest = reYield.ReplaceAllStringFunc(rest, func(m string) string {
		sm := reYield.FindStringSubmatch(m)
		if len(sm) >= 3 {
			yieldName := normalizeName(sm[1])
			p.Yields[yieldName] = sm[2]
			return fmt.Sprintf(`{{ template "%s%s" . }}`, sectionNamePrefix, yieldName)
		}
		return m
	})

	// convert @stack to template inclusion: @stack('name') => {{ template "__stack_name" . }}
	rest = reStack.ReplaceAllStringFunc(rest, func(m string) string {
		sm := reStack.FindStringSubmatch(m)
		if len(sm) >= 2 {
			stackName := normalizeName(sm[1])
			p.Stacks[stackName] = struct{}{}
			return fmt.Sprintf(`{{ template "%s%s" . }}`, stackNamePrefix, stackName)
		}
		return m
	})

	// process includes: @include('partial') -> {{ template "__include_partial" . }}
	rest = replaceDirectiveCalls(rest, "include", func(args []string) (string, bool) {
		if len(args) == 0 {
			return "", false
		}
		partialName, ok := parseQuotedDirectiveName(args[0])
		if !ok {
			return "", false
		}
		pipeline := "."
		if len(args) > 1 {
			pipeline = strings.TrimSpace(args[1])
			if pipeline == "" {
				pipeline = "."
			}
		}
		p.Includes[partialName] = struct{}{}
		return fmt.Sprintf(`{{ template "%s%s" %s }}`, partialNamePrefix, partialName, pipeline), true
	})

	// Parse sections
	for {
		start := strings.Index(rest, "@section(")
		if start == -1 {
			break
		}

		callEnd, args, ok := parseDirectiveCall(rest, start, "section")
		if !ok || len(args) == 0 {
			start++
			if start >= len(rest) {
				break
			}
			rest = rest[:start] + rest[start:]
			continue
		}

		sectionName, ok := parseQuotedDirectiveName(args[0])
		if !ok {
			continue
		}

		if len(args) > 1 {
			//	@section('name',	content expression)
			p.Sections[sectionName] = strings.TrimSpace(args[1])
			rest = rest[:start] + rest[callEnd:]
			continue
		}

		// find end
		endIdx := reSectionEnd.FindStringIndex(rest[callEnd:])
		if endIdx == nil {
			return nil, fmt.Errorf("[%s] missing @endsection", p.Name)
		}
		contentStart := callEnd
		contentEnd := callEnd + endIdx[0]
		p.Sections[sectionName] = strings.TrimSpace(rest[contentStart:contentEnd])
		// remove the section from rest by replacing with empty string
		rest = rest[:start] + rest[contentEnd+len("@endsection"):] // remove tail including @endsection
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
		p.PushStacks[stackName] = append(p.PushStacks[stackName], strings.TrimSpace(rest[contentStart:contentEnd]))
		// remove the section from rest by replacing with empty string
		rest = rest[:loc[0]] + rest[contentEnd+len("@endpush"):] // remove tail including @endpush
	}

	p.StandaloneBody = strings.TrimSpace(rest)

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
	var result strings.Builder
	for name, info := range ctx.Yields {
		if _, ok := ctx.FilledSections[name]; !ok {
			result.WriteString("\n")
			result.WriteString("{{ define \"")
			result.WriteString(sectionNamePrefix)
			result.WriteString(name)
			result.WriteString("\" }}")
			result.WriteString(info.Default)
			result.WriteString("{{ end }}")
		}
	}
	return result.String()
}

// normalizeName: remove quotes/spaces and extensions, normalize slashes
func normalizeName(n string) string {
	n = strings.TrimSpace(n)
	n = strings.Trim(n, `"' `)
	n = strings.ReplaceAll(n, ".", "/")
	n = filepath.ToSlash(n)
	return n
}

func replaceDirectiveCalls(input string, directive string, replacer func(args []string) (string, bool)) string {
	marker := "@" + directive + "("
	var out strings.Builder
	cursor := 0

	for {
		rel := strings.Index(input[cursor:], marker)
		if rel == -1 {
			out.WriteString(input[cursor:])
			break
		}

		start := cursor + rel
		out.WriteString(input[cursor:start])

		end, args, ok := parseDirectiveCall(input, start, directive)
		if !ok {
			out.WriteString(input[start : start+1])
			cursor = start + 1
			continue
		}

		replacement, ok := replacer(args)
		if !ok {
			out.WriteString(input[start:end])
		} else {
			out.WriteString(replacement)
		}
		cursor = end
	}

	return out.String()
}

func parseDirectiveCall(input string, start int, directive string) (int, []string, bool) {
	marker := "@" + directive + "("
	if start < 0 || start >= len(input) || !strings.HasPrefix(input[start:], marker) {
		return 0, nil, false
	}

	argStart := start + len(marker)
	depth := 1
	inSingle := false
	inDouble := false
	escaped := false

	for i := argStart; i < len(input); i++ {
		ch := input[i]

		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' && (inSingle || inDouble) {
			escaped = true
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if inSingle || inDouble {
			continue
		}

		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				argsText := input[argStart:i]
				return i + 1, splitTopLevelArgs(argsText), true
			}
		}
	}

	return 0, nil, false
}

func splitTopLevelArgs(argsText string) []string {
	trimmed := strings.TrimSpace(argsText)
	if trimmed == "" {
		return nil
	}

	args := []string{}
	start := 0
	depth := 0
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(argsText); i++ {
		ch := argsText[i]

		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' && (inSingle || inDouble) {
			escaped = true
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if inSingle || inDouble {
			continue
		}

		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				arg := strings.TrimSpace(argsText[start:i])
				if arg != "" {
					args = append(args, arg)
				}
				start = i + 1
			}
		}
	}

	if arg := strings.TrimSpace(argsText[start:]); arg != "" {
		args = append(args, arg)
	}

	return args
}

func parseQuotedDirectiveName(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) < 2 {
		return "", false
	}
	if (trimmed[0] != '\'' && trimmed[0] != '"') || trimmed[len(trimmed)-1] != trimmed[0] {
		return "", false
	}
	return normalizeName(trimmed[1 : len(trimmed)-1]), true
}
