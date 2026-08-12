package agent

import (
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

// readTOMLMap reads a TOML file into a map[string]any.
// If the file does not exist, an empty map is returned without error.
func readTOMLMap(filePath string) (map[string]any, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return make(map[string]any), nil
	}
	if err != nil {
		return nil, fmt.Errorf("agent: read toml %s: %w", filePath, err)
	}

	var res map[string]any
	if err := toml.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("agent: unmarshal toml %s: %w", filePath, err)
	}
	if res == nil {
		res = make(map[string]any)
	}
	return res, nil
}

// updateTOMLProvider updates a TOML file at filePath.
// If section and providerID are non-empty, providerFields are merged into targetMap[section][providerID].
// rootFields are merged into targetMap.
func updateTOMLProvider(filePath string, section string, providerID string, providerFields map[string]any, rootFields map[string]any) error {
	m, err := readTOMLMap(filePath)
	if err != nil {
		return err
	}

	for k, v := range rootFields {
		m[k] = v
	}

	if section != "" && providerID != "" {
		var secMap map[string]any
		if existingSec, ok := m[section].(map[string]any); ok && existingSec != nil {
			secMap = existingSec
		} else {
			secMap = make(map[string]any)
		}

		var provMap map[string]any
		if existingProv, ok := secMap[providerID].(map[string]any); ok && existingProv != nil {
			provMap = existingProv
		} else {
			provMap = make(map[string]any)
		}

		for k, v := range providerFields {
			provMap[k] = v
		}
		secMap[providerID] = provMap
		m[section] = secMap
	}

	data, err := toml.Marshal(m)
	if err != nil {
		return fmt.Errorf("agent: marshal toml %s: %w", filePath, err)
	}

	return atomicWrite(filePath, data, 0600)
}

// resetTOMLProvider removes specified provider and/or root keys from a TOML file at filePath.
func resetTOMLProvider(filePath string, section string, providerID string, removeRootKeys []string) error {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("agent: read toml %s: %w", filePath, err)
	}

	var m map[string]any
	if err := toml.Unmarshal(data, &m); err != nil || m == nil {
		return nil
	}

	for _, k := range removeRootKeys {
		delete(m, k)
	}

	if section != "" && providerID != "" {
		if secMap, ok := m[section].(map[string]any); ok && secMap != nil {
			delete(secMap, providerID)
			if len(secMap) == 0 {
				delete(m, section)
			}
		}
	}

	buf, err := toml.Marshal(m)
	if err != nil {
		return fmt.Errorf("agent: marshal toml %s: %w", filePath, err)
	}

	return atomicWrite(filePath, buf, 0600)
}
