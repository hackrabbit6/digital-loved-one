//go:build objectbox

package memory

import "fmt"

// NewStore creates a local memory store by engine name.
func NewStore(engine, basePath string) (Store, error) {
	switch engine {
	case "", "json":
		return NewGraphStore(basePath)
	case "objectbox":
		return NewObjectBoxStore(basePath)
	default:
		return nil, fmt.Errorf("unsupported memory store engine %q", engine)
	}
}
