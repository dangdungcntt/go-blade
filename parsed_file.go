package blade

import (
	"errors"
)

type CompileContext struct {
	Yields       map[string]string
	FilledYields map[string]struct{}
	Files        map[string]*ParsedFile
}

type ParsedFile struct {
	// Raw is the raw file content
	Raw string
	// Extends is the file to extend
	Extends string
	// Includes is a list of files to include
	Includes []string
	// Sections is a map of section names to content
	Sections map[string]string
	// Yields is a map of section names to default content
	Yields map[string]string
	// StandaloneBody is the body of the file without sections and includes
	StandaloneBody string
	// ParsedAt is the time when the file was parsed in unix milliseconds
	ParsedAt int64
}

// ToTemplateString converts the parsed file to a template string.
func (p *ParsedFile) ToTemplateString(ctx *CompileContext) (string, error) {
	var result string
	for name, s := range p.Sections {
		if _, ok := ctx.FilledYields[name]; ok {
			continue
		}
		result += `{{ define "` + name + `" }}` + s + `{{ end }}`
		ctx.FilledYields[name] = struct{}{}
	}

	if p.Extends != "" {
		parent, found := ctx.Files[p.Extends]
		if !found {
			return "", errors.New("extends not found: " + p.Extends)
		}
		templateText, err := parent.ToTemplateString(ctx)
		if err != nil {
			return "", err
		}
		result += templateText
	}

	for _, include := range p.Includes {
		partial, found := ctx.Files[include]
		if !found {
			return "", errors.New("include not found: " + include)
		}
		templateText, err := partial.ToTemplateString(ctx)
		if err != nil {
			return "", err
		}
		result += `{{ define "` + include + `" }}` + templateText + `{{ end }}`
	}

	for name, defaultValue := range p.Yields {
		if _, ok := ctx.Yields[name]; ok {
			return "", errors.New("yield already defined: " + name)
		}
		ctx.Yields[name] = defaultValue
	}

	if p.Extends == "" {
		result += p.StandaloneBody
	}

	return result, nil
}
