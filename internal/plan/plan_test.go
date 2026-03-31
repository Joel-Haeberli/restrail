package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndParseWithParamMappings(t *testing.T) {
	dir := t.TempDir()

	mappings := map[string]string{
		"number":     "$",
		"buildingId": "buildings",
		"floorId":    "floors",
	}

	phases := []Phase{
		{
			Name: "Phase 1: Create",
			Steps: []Step{
				{Filename: "buildings_POST.request", Annotations: []string{"EXTRACT_ID"}},
				{Filename: "floors_POST.request", Annotations: []string{"EXTRACT_ID"}},
			},
		},
	}

	err := Write(dir, "custom", phases, []string{"buildings", "floors"}, nil, mappings)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	parsed, err := Parse(filepath.Join(dir, "execution.plan"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if parsed.Profile != "custom" {
		t.Errorf("profile = %q, want %q", parsed.Profile, "custom")
	}
	if len(parsed.ParamMappings) != 3 {
		t.Fatalf("expected 3 param mappings, got %d", len(parsed.ParamMappings))
	}
	if parsed.ParamMappings["number"] != "$" {
		t.Errorf("number = %q, want %q", parsed.ParamMappings["number"], "$")
	}
	if parsed.ParamMappings["buildingId"] != "buildings" {
		t.Errorf("buildingId = %q, want %q", parsed.ParamMappings["buildingId"], "buildings")
	}
	if parsed.ParamMappings["floorId"] != "floors" {
		t.Errorf("floorId = %q, want %q", parsed.ParamMappings["floorId"], "floors")
	}
}

func TestParseWithoutParamMappings(t *testing.T) {
	dir := t.TempDir()

	phases := []Phase{
		{Name: "Phase 1", Steps: []Step{{Filename: "test_POST.request"}}},
	}

	err := Write(dir, "ddd", phases, []string{"test"}, nil, nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	parsed, err := Parse(filepath.Join(dir, "execution.plan"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if parsed.ParamMappings != nil {
		t.Errorf("expected nil ParamMappings, got %v", parsed.ParamMappings)
	}
}

func TestWrittenPlanContainsParamMappingLines(t *testing.T) {
	dir := t.TempDir()

	mappings := map[string]string{"buildingId": "buildings"}
	phases := []Phase{{Name: "Phase 1", Steps: []Step{{Filename: "test.request"}}}}

	err := Write(dir, "test", phases, []string{"test"}, nil, mappings)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "execution.plan"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)
	expected := "# @param_mapping buildingId=buildings"
	if !contains(content, expected) {
		t.Errorf("plan file missing %q:\n%s", expected, content)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
