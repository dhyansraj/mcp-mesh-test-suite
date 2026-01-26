package api

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// TestInfo represents a discovered test case
type TestInfo struct {
	TestID      string   `json:"test_id"`
	UseCase     string   `json:"use_case"`
	TestCase    string   `json:"test_case"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// UseCaseInfo represents a use case with its tests
type UseCaseInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TestCount int    `json:"test_count"`
}

// DiscoverTests finds all tests in a suite folder
func DiscoverTests(suitePath string) ([]TestInfo, []UseCaseInfo, error) {
	suitesDir := filepath.Join(suitePath, "suites")

	if _, err := os.Stat(suitesDir); os.IsNotExist(err) {
		return []TestInfo{}, []UseCaseInfo{}, nil
	}

	var tests []TestInfo
	useCaseMap := make(map[string]*UseCaseInfo)

	// List use case directories
	ucEntries, err := os.ReadDir(suitesDir)
	if err != nil {
		return nil, nil, err
	}

	for _, ucEntry := range ucEntries {
		if !ucEntry.IsDir() || ucEntry.Name()[0] == '.' {
			continue
		}

		ucName := ucEntry.Name()
		ucPath := filepath.Join(suitesDir, ucName)

		// List test case directories
		tcEntries, err := os.ReadDir(ucPath)
		if err != nil {
			continue
		}

		for _, tcEntry := range tcEntries {
			if !tcEntry.IsDir() || tcEntry.Name()[0] == '.' {
				continue
			}

			tcName := tcEntry.Name()
			testYamlPath := filepath.Join(ucPath, tcName, "test.yaml")

			// Check if test.yaml exists
			if _, err := os.Stat(testYamlPath); os.IsNotExist(err) {
				continue
			}

			// Parse test.yaml
			testConfig, err := parseTestYaml(testYamlPath)
			if err != nil {
				// Skip invalid test files
				continue
			}

			testID := ucName + "/" + tcName
			test := TestInfo{
				TestID:      testID,
				UseCase:     ucName,
				TestCase:    tcName,
				Name:        getStringOr(testConfig, "name", tcName),
				Description: getStringOr(testConfig, "description", ""),
				Tags:        getStringSlice(testConfig, "tags"),
			}
			tests = append(tests, test)

			// Track use case
			if _, ok := useCaseMap[ucName]; !ok {
				useCaseMap[ucName] = &UseCaseInfo{
					ID:   ucName,
					Name: ucName,
				}
			}
			useCaseMap[ucName].TestCount++
		}
	}

	// Sort tests by ID
	sort.Slice(tests, func(i, j int) bool {
		return tests[i].TestID < tests[j].TestID
	})

	// Convert use cases to slice
	useCases := make([]UseCaseInfo, 0, len(useCaseMap))
	for _, uc := range useCaseMap {
		useCases = append(useCases, *uc)
	}
	sort.Slice(useCases, func(i, j int) bool {
		return useCases[i].ID < useCases[j].ID
	})

	return tests, useCases, nil
}

func parseTestYaml(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config, nil
}

func getStringOr(m map[string]any, key, defaultVal string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

func getStringSlice(m map[string]any, key string) []string {
	if v, ok := m[key]; ok {
		if slice, ok := v.([]any); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}
