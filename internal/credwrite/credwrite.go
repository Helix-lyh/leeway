// Package credwrite patches YAML credentials files (e.g. DeepSeek Harness's
// $DSH_HOME/.credentials.yaml) in place: one credential reference → value
// mapping, with the existing document's order and comments preserved.
package credwrite

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Patch sets key to value in the YAML document at path (creating the file
// with 0600 permissions if absent) and writes it back atomically. Any value
// is accepted, matching the "any string value round-trips" contract of the
// DSH credentials file; the root must be a mapping or the file is refused.
func Patch(path, key, value string) error {
	root, err := load(path)
	if err != nil {
		return err
	}
	set(root, key, value)
	out, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// load parses the document into a mapping node; a missing file is an empty
// mapping, a non-mapping root (or unparseable YAML) is an error so a corrupt
// credentials file is never silently overwritten.
func load(path string) (*yaml.Node, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}
	root := &yaml.Node{}
	if err := yaml.Unmarshal(raw, root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Unmarshalling into a Node wraps the real root in a Document node.
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s is not a YAML mapping", path)
	}
	return root, nil
}

// Lookup returns the string value of key in the YAML mapping at path.
// Missing files, missing keys, and unreadable documents yield "".
func Lookup(path, key string) string {
	root, err := load(path)
	if err != nil {
		return ""
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1].Value
		}
	}
	return ""
}

// set updates an existing key's value node in place or appends a new pair,
// preserving document order and any comments.
func set(root *yaml.Node, key, value string) {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			v := root.Content[i+1]
			v.Kind = yaml.ScalarNode
			v.Tag = "!!str"
			v.Value = value
			v.Style = 0
			return
		}
	}
	k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	v := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	root.Content = append(root.Content, k, v)
}
