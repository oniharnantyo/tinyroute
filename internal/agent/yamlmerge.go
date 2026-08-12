package agent

import (
	"fmt"
	"os"

	yaml "gopkg.in/yaml.v3"
)

// readYAMLMap reads a YAML file into a map[string]any.
// If the file does not exist, an empty map is returned without error.
func readYAMLMap(filePath string) (map[string]any, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return make(map[string]any), nil
	}
	if err != nil {
		return nil, fmt.Errorf("agent: read yaml %s: %w", filePath, err)
	}

	var res map[string]any
	if err := yaml.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("agent: unmarshal yaml %s: %w", filePath, err)
	}
	if res == nil {
		res = make(map[string]any)
	}
	return res, nil
}

// updateYAMLMap updates a YAML file at filePath by merging setFields into the root map.
func updateYAMLMap(filePath string, setFields map[string]any) error {
	m, err := readYAMLMap(filePath)
	if err != nil {
		return err
	}

	for k, v := range setFields {
		m[k] = v
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("agent: marshal yaml %s: %w", filePath, err)
	}

	return atomicWrite(filePath, data, 0600)
}

// resetYAMLKeys removes removeKeys from a YAML file at filePath.
func resetYAMLKeys(filePath string, removeKeys []string) error {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("agent: read yaml %s: %w", filePath, err)
	}

	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil || m == nil {
		return nil
	}

	for _, k := range removeKeys {
		delete(m, k)
	}

	buf, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("agent: marshal yaml %s: %w", filePath, err)
	}

	return atomicWrite(filePath, buf, 0600)
}
