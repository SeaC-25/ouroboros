package patterns

type Pattern struct {
	Name       string            `yaml:"name"`
	Version    string            `yaml:"version"`
	Metadata   map[string]string `yaml:"metadata"`
	MaxSteps   int               `yaml:"max_steps"`
	Nodes      []Node            `yaml:"nodes"`
	Edges      []Edge            `yaml:"edges"`
	Conditions []Condition       `yaml:"conditions"`
}

type Node struct {
	ID       string `yaml:"id"`
	Kind     string `yaml:"kind"`
	Terminal bool   `yaml:"terminal"`
}

type Edge struct {
	From      string `yaml:"from"`
	To        string `yaml:"to"`
	Condition string `yaml:"condition"`
}

type Condition struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}
