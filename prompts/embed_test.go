package prompts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiscoverySchemaUsesStrictObjects(t *testing.T) {
	_, schema, err := DiscoveryV1()
	if err != nil {
		t.Fatalf("DiscoveryV1() error = %v", err)
	}
	var root any
	if err := json.Unmarshal(schema, &root); err != nil {
		t.Fatalf("schema JSON error = %v", err)
	}
	checkStrictObjects(t, root, "$")
}

func TestPromptsTreatRepositoryContentAsUntrustedEvidence(t *testing.T) {
	loaders := []struct {
		name string
		load func() (string, []byte, error)
	}{{"discovery", DiscoveryV1}, {"specialist", SpecialistV1}, {"critic", CriticV1}}
	for _, item := range loaders {
		t.Run(item.name, func(t *testing.T) {
			system, _, err := item.load()
			if err != nil {
				t.Fatalf("load prompt: %v", err)
			}
			if !strings.Contains(system, "untrusted evidence") || !strings.Contains(system, "grant tools") || !strings.Contains(system, "application policy") {
				t.Fatalf("prompt does not preserve the repository trust boundary: %q", system)
			}
		})
	}
}

func TestCriticSchemaUsesStrictObjects(t *testing.T) {
	_, schema, err := CriticV1()
	if err != nil {
		t.Fatalf("CriticV1() error = %v", err)
	}
	var root any
	if err := json.Unmarshal(schema, &root); err != nil {
		t.Fatalf("schema JSON error = %v", err)
	}
	checkStrictObjects(t, root, "$")
}

func checkStrictObjects(t *testing.T, value any, path string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if typed["type"] == "object" {
			if additional, exists := typed["additionalProperties"]; !exists || additional != false {
				t.Errorf("%s object is not strict", path)
			}
			properties, _ := typed["properties"].(map[string]any)
			required, _ := typed["required"].([]any)
			if len(properties) != len(required) {
				t.Errorf("%s requires %d of %d properties", path, len(required), len(properties))
			}
		}
		for key, child := range typed {
			checkStrictObjects(t, child, path+"."+key)
		}
	case []any:
		for index, child := range typed {
			checkStrictObjects(t, child, path+"[]"+string(rune(index)))
		}
	}
}
