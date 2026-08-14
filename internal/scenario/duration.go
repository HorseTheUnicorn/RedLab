package scenario

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a YAML-friendly time.Duration. YAML authors can use values such
// as 30m, 60m, or 2h while Go code still gets ordinary time.Duration values.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a scalar")
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) String() string          { return time.Duration(d).String() }
func (d Duration) Duration() time.Duration { return time.Duration(d) }
