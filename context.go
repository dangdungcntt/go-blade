package blade

type CompileContext struct {
	Files map[string]*ParsedFile
	// Yields is a map of section names to default content
	Yields       map[string]YieldInfo
	FilledYields map[string]struct{}
	// Stacks is a map of stack names to a template file, prevent duplicate stack names
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
