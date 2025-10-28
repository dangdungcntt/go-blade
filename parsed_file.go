package blade

import (
	"fmt"
	"strings"
)

type ParsedFile struct {
	Name string
	// Raw is the raw file content
	Raw string
	// Extends is the file to extend
	Extends string
	// Includes is a list of files to include
	Includes []string
	// Yields is a map of section names to default content
	Yields map[string]string
	// Sections is a map of section names to content
	Sections map[string]string
	// Stacks is a map of stack names
	Stacks map[string]struct{}
	// PushStacks is a map of stack names to values to push
	PushStacks map[string][]string
	// StandaloneBody is the body of the file without sections and includes
	StandaloneBody string
	// ParsedAt is the time when the file was parsed in unix milliseconds
	ParsedAt int64
}

// ToTemplateString converts the parsed file to a template string.
func (p *ParsedFile) ToTemplateString(ctx *CompileContext) (string, error) {
	var result strings.Builder

	for stackName, values := range p.PushStacks {
		// We need push to stack in reverse order, since we are compiling from child to parent
		size := len(values)
		for i := range values {
			ctx.PushStacks[stackName] = append(ctx.PushStacks[stackName], values[size-1-i])
		}
	}

	for name := range p.Stacks {
		if fileName, ok := ctx.Stacks[name]; ok {
			return "", fmt.Errorf(`[%s] duplicate stack name "%s", already defined in file "%s"`, p.Name, name, fileName)
		}
		ctx.Stacks[name] = p.Name
		result.WriteString("\n{{ define \"")
		result.WriteString(stackNamePrefix)
		result.WriteString(name)
		result.WriteString("\" }}")
		// Pop from stack
		size := len(ctx.PushStacks[name])
		for i := range ctx.PushStacks[name] {
			result.WriteString(ctx.PushStacks[name][size-1-i])
		}
		result.WriteString("{{ end }}")
	}

	for name, s := range p.Sections {
		if _, ok := ctx.FilledYields[name]; ok {
			continue
		}
		result.WriteString("\n{{ define \"")
		result.WriteString(yieldNamePrefix)
		result.WriteString(name)
		result.WriteString("\" }}")
		result.WriteString(s)
		result.WriteString("{{ end }}")
		ctx.FilledYields[name] = struct{}{}
	}

	for name, defaultValue := range p.Yields {
		if info, ok := ctx.Yields[name]; ok {
			return "", fmt.Errorf(`[%s] duplicate yield name "%s", already defined in file "%s"`, p.Name, name, info.FileName)
		}
		ctx.Yields[name] = YieldInfo{
			Name:     name,
			FileName: p.Name,
			Default:  defaultValue,
		}
	}

	if p.Extends != "" {
		parent, found := ctx.Files[p.Extends]
		if !found {
			return "", fmt.Errorf(`[%s] template "%s" not found to extends`, p.Name, p.Extends)
		}
		templateText, err := parent.ToTemplateString(ctx)
		if err != nil {
			return "", err
		}
		result.WriteString(templateText)
	}

	for _, partialName := range p.Includes {
		partial, found := ctx.Files[partialName]
		if !found {
			return "", fmt.Errorf(`[%s] template "%s" not found to include`, p.Name, partialName)
		}
		templateText, err := partial.ToTemplateString(ctx)
		if err != nil {
			return "", err
		}
		result.WriteString("\n{{ define \"")
		result.WriteString(partialNamePrefix)
		result.WriteString(partialName)
		result.WriteString("\" }}")
		result.WriteString(templateText)
		result.WriteString("{{ end }}")
	}

	if p.Extends == "" {
		result.WriteString("\n")
		result.WriteString(p.StandaloneBody)
	}

	return result.String(), nil
}
