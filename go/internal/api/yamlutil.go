package api

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// YAMLDocument wraps a yaml.Node to provide comment-preserving operations
type YAMLDocument struct {
	Root *yaml.Node
}

// LoadYAMLFile loads a YAML file preserving comments and key ordering
func LoadYAMLFile(path string) (*YAMLDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	return &YAMLDocument{Root: &root}, nil
}

// SaveYAMLFile saves the YAML document preserving comments and key ordering
func (d *YAMLDocument) SaveYAMLFile(path string) error {
	data, err := d.ToBytes()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ToBytes converts the document to YAML bytes with 2-space indentation
func (d *YAMLDocument) ToBytes() ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(d.Root); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// GetContent returns the content node (skips document node)
func (d *YAMLDocument) GetContent() *yaml.Node {
	if d.Root.Kind == yaml.DocumentNode && len(d.Root.Content) > 0 {
		return d.Root.Content[0]
	}
	return d.Root
}

// ToMap converts the YAML to a map for JSON serialization
func (d *YAMLDocument) ToMap() (map[string]any, error) {
	var result map[string]any
	data, err := yaml.Marshal(d.Root)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SetField sets a top-level field value, preserving position if it exists
func (d *YAMLDocument) SetField(key string, value any) error {
	content := d.GetContent()
	if content.Kind != yaml.MappingNode {
		return fmt.Errorf("root is not a mapping")
	}

	// Find existing key
	for i := 0; i < len(content.Content); i += 2 {
		if content.Content[i].Value == key {
			// Update existing value
			newValue := &yaml.Node{}
			if err := newValue.Encode(value); err != nil {
				return err
			}
			content.Content[i+1] = newValue
			return nil
		}
	}

	// Add new key-value pair at the end
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valueNode := &yaml.Node{}
	if err := valueNode.Encode(value); err != nil {
		return err
	}
	content.Content = append(content.Content, keyNode, valueNode)
	return nil
}

// GetField gets a top-level field value
func (d *YAMLDocument) GetField(key string) *yaml.Node {
	content := d.GetContent()
	if content.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(content.Content); i += 2 {
		if content.Content[i].Value == key {
			return content.Content[i+1]
		}
	}
	return nil
}

// GetSequence gets a sequence (array) field
func (d *YAMLDocument) GetSequence(key string) []*yaml.Node {
	node := d.GetField(key)
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	return node.Content
}

// UpdateSequenceItem updates an item in a sequence at the given index
func (d *YAMLDocument) UpdateSequenceItem(key string, index int, updates map[string]any) error {
	node := d.GetField(key)
	if node == nil || node.Kind != yaml.SequenceNode {
		return fmt.Errorf("field %s is not a sequence", key)
	}

	if index < 0 || index >= len(node.Content) {
		return fmt.Errorf("index %d out of range", index)
	}

	item := node.Content[index]
	if item.Kind != yaml.MappingNode {
		return fmt.Errorf("item at index %d is not a mapping", index)
	}

	// Update fields in the item
	for k, v := range updates {
		if err := updateNodeField(item, k, v); err != nil {
			return err
		}
	}

	return nil
}

// AddSequenceItem adds an item to a sequence
func (d *YAMLDocument) AddSequenceItem(key string, item map[string]any, index *int) error {
	node := d.GetField(key)

	// Create sequence if it doesn't exist
	if node == nil {
		content := d.GetContent()
		if content.Kind != yaml.MappingNode {
			return fmt.Errorf("root is not a mapping")
		}

		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
		node = &yaml.Node{Kind: yaml.SequenceNode}
		content.Content = append(content.Content, keyNode, node)
	}

	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("field %s is not a sequence", key)
	}

	// Create new item node
	newItem := &yaml.Node{}
	if err := newItem.Encode(item); err != nil {
		return err
	}

	// Insert at index or append
	if index != nil && *index < len(node.Content) {
		// Insert at index
		idx := *index
		node.Content = append(node.Content[:idx], append([]*yaml.Node{newItem}, node.Content[idx:]...)...)
	} else {
		// Append
		node.Content = append(node.Content, newItem)
	}

	return nil
}

// RemoveSequenceItem removes an item from a sequence at the given index
func (d *YAMLDocument) RemoveSequenceItem(key string, index int) error {
	node := d.GetField(key)
	if node == nil || node.Kind != yaml.SequenceNode {
		return fmt.Errorf("field %s is not a sequence", key)
	}

	if index < 0 || index >= len(node.Content) {
		return fmt.Errorf("index %d out of range", index)
	}

	node.Content = append(node.Content[:index], node.Content[index+1:]...)
	return nil
}

// MergeUpdates merges updates into the document preserving structure
func (d *YAMLDocument) MergeUpdates(updates map[string]any) error {
	content := d.GetContent()
	if content.Kind != yaml.MappingNode {
		return fmt.Errorf("root is not a mapping")
	}

	for key, value := range updates {
		// Handle delete marker
		if strVal, ok := value.(string); ok && strVal == "__DELETE__" {
			removeNodeField(content, key)
			continue
		}

		if err := updateNodeField(content, key, value); err != nil {
			return err
		}
	}

	return nil
}

// updateNodeField updates a field in a mapping node, preserving position
// For nested maps, it recursively merges instead of replacing
func updateNodeField(node *yaml.Node, key string, value any) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("node is not a mapping")
	}

	// Find existing key
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			existingVal := node.Content[i+1]

			// If both existing and new values are maps, merge recursively
			if nestedMap, ok := value.(map[string]any); ok && existingVal.Kind == yaml.MappingNode {
				for k, v := range nestedMap {
					// Handle delete marker in nested map
					if strVal, ok := v.(string); ok && strVal == "__DELETE__" {
						removeNodeField(existingVal, k)
						continue
					}
					if err := updateNodeField(existingVal, k, v); err != nil {
						return err
					}
				}
				return nil
			}

			// Otherwise replace the value entirely, preserving inline comment
			newValue := &yaml.Node{}
			if err := newValue.Encode(value); err != nil {
				return err
			}
			// Preserve inline comment from existing value
			newValue.LineComment = existingVal.LineComment
			node.Content[i+1] = newValue
			return nil
		}
	}

	// Add new key-value pair
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valueNode := &yaml.Node{}
	if err := valueNode.Encode(value); err != nil {
		return err
	}
	node.Content = append(node.Content, keyNode, valueNode)
	return nil
}

// removeNodeField removes a field from a mapping node
func removeNodeField(node *yaml.Node, key string) {
	if node.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}
