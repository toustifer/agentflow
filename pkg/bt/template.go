package bt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TreeSource holds a deserialized tree along with its original JSON source.
type TreeSource struct {
	Name   string
	Source json.RawMessage
}

// LoadTreeDir scans a directory for .json tree files, deserializes each,
// and registers them in the tree Registry. Returns the list of loaded sources
// for inspection tools.
//
// Each .json file must be a valid TreeFile format (name + tree).
func LoadTreeDir(dir string, reg *FactoryRegistry, treeReg *Registry) ([]TreeSource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sources []TreeSource
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}

		// Validate JSON before trying to deserialize
		if !json.Valid(data) {
			return nil, fmt.Errorf("invalid JSON in %s", entry.Name())
		}

		node, _, err := DeserializeTree(data, reg)
		if err != nil {
			return nil, fmt.Errorf("deserialize %s: %w", entry.Name(), err)
		}

		// Use filename without extension as tree name
		name := entry.Name()[:len(entry.Name())-5]
		treeReg.Register(name, node)

		sources = append(sources, TreeSource{
			Name:   name,
			Source: data,
		})
	}

	return sources, nil
}

// ValidateTreeJSON parses and builds a tree from JSON bytes without
// registering it. Returns nil on success, or an error describing the issue.
func ValidateTreeJSON(data []byte, reg *FactoryRegistry) error {
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON")
	}
	_, _, err := DeserializeTree(data, reg)
	return err
}
