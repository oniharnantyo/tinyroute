package agent

import (
	"encoding/json"
	"fmt"
	"os"
)

// readJSONMap reads a JSON file into a map[string]any.
// If the file does not exist, an empty map is returned without error.
func readJSONMap(filePath string) (map[string]any, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return make(map[string]any), nil
	}
	if err != nil {
		return nil, fmt.Errorf("agent: read json %s: %w", filePath, err)
	}

	var res map[string]any
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("agent: unmarshal json %s: %w", filePath, err)
	}
	if res == nil {
		res = make(map[string]any)
	}
	return res, nil
}

// updateJSONEnv updates a JSON file at filePath.
// If envKey is non-empty, setFields are merged into targetMap[envKey].
// rootFields are merged into targetMap directly.
func updateJSONEnv(filePath string, envKey string, setFields map[string]any, rootFields map[string]any) error {
	m, err := readJSONMap(filePath)
	if err != nil {
		return err
	}

	for k, v := range rootFields {
		m[k] = v
	}

	if envKey != "" && len(setFields) > 0 {
		var envMap map[string]any
		if existing, ok := m[envKey].(map[string]any); ok && existing != nil {
			envMap = existing
		} else {
			envMap = make(map[string]any)
		}
		for k, v := range setFields {
			envMap[k] = v
		}
		m[envKey] = envMap
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("agent: marshal json %s: %w", filePath, err)
	}
	data = append(data, '\n')

	return atomicWrite(filePath, data, 0600)
}

// resetJSONEnv removes specified keys from a JSON file at filePath.
// If envKey is non-empty, removeEnvKeys are removed from targetMap[envKey].
// removeRootKeys are removed from targetMap.
func resetJSONEnv(filePath string, envKey string, removeEnvKeys []string, removeRootKeys []string) error {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("agent: read json %s: %w", filePath, err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return nil
	}

	for _, k := range removeRootKeys {
		delete(m, k)
	}

	if envKey != "" {
		if envMap, ok := m[envKey].(map[string]any); ok && envMap != nil {
			for _, k := range removeEnvKeys {
				delete(envMap, k)
			}
			if len(envMap) == 0 {
				delete(m, envKey)
			}
		}
	}

	buf, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("agent: marshal json %s: %w", filePath, err)
	}
	buf = append(buf, '\n')

	return atomicWrite(filePath, buf, 0600)
}

func mustMarshalJSON(v any) []byte {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return append(data, '\n')
}
