package runner

// Framework describes a test framework and how to detect/run it.
type Framework struct {
	Name      string
	DetectFn  func(workDir string) bool
	Command   string
	Args      []string
}

// KnownFrameworks lists test frameworks in detection priority order.
var KnownFrameworks = []Framework{
	{
		Name:    "go",
		Command: "go",
		Args:    []string{"test", "./..."},
	},
	{
		Name:    "npm",
		Command: "npm",
		Args:    []string{"test"},
	},
	{
		Name:    "pytest",
		Command: "pytest",
		Args:    nil,
	},
	{
		Name:    "maven",
		Command: "mvn",
		Args:    []string{"test"},
	},
	{
		Name:    "gradle",
		Command: "gradle",
		Args:    []string{"test"},
	},
	{
		Name:    "cargo",
		Command: "cargo",
		Args:    []string{"test"},
	},
	{
		Name:    "make",
		Command: "make",
		Args:    []string{"test"},
	},
}
