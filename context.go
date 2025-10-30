package blade

const (
	sectionNamePrefix = "__section_"
	stackNamePrefix   = "__stack_"
	partialNamePrefix = "__partial_"
)

type CompileContext struct {
	Files map[string]*ParsedFile
	// Yields maps yield names to their default content and prevents duplicate yield names.
	Yields map[string]YieldInfo
	// FilledSections is a map of section names, it prevents override section content from parent layout
	FilledSections map[string]struct{}
	// FilledIncludes is a map of partial names, it prevents duplicate partial names
	FilledIncludes map[string]struct{}
	// Stacks is a map of stack names to a template file, it prevents duplicate stack names and provides friendly error messages
	Stacks map[string]string
	// PushStacks is a map of stack names to values to push
	// In the array, the last value is popped first
	PushStacks map[string][]string
}

// YieldInfo contains information about a yield
type YieldInfo struct {
	Name     string
	FileName string
	Default  string
}
