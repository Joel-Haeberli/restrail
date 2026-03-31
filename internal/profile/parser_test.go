package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseParamMappings(t *testing.T) {
	content := `-----------
Definition
-----------

OP1: POST '/{domain}' SEND_ONE READ_ONE 201: Create

-----------
Execution
-----------

OP1

-----------
Params
-----------

# A comment
number = $
buildingId = buildings
floorId = floors
`
	p, err := Parse("test", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.ParamMappings) != 3 {
		t.Fatalf("expected 3 param mappings, got %d", len(p.ParamMappings))
	}
	if p.ParamMappings["number"] != "$" {
		t.Errorf("number mapping = %q, want %q", p.ParamMappings["number"], "$")
	}
	if p.ParamMappings["buildingId"] != "buildings" {
		t.Errorf("buildingId mapping = %q, want %q", p.ParamMappings["buildingId"], "buildings")
	}
	if p.ParamMappings["floorId"] != "floors" {
		t.Errorf("floorId mapping = %q, want %q", p.ParamMappings["floorId"], "floors")
	}
}

func TestParseNoParamsSection(t *testing.T) {
	content := `-----------
Definition
-----------

OP1: POST '/{domain}' SEND_ONE READ_ONE 201: Create

-----------
Execution
-----------

OP1
`
	p, err := Parse("test", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ParamMappings != nil {
		t.Errorf("expected nil ParamMappings, got %v", p.ParamMappings)
	}
}

func TestParseInvalidParamMapping(t *testing.T) {
	content := `-----------
Definition
-----------

OP1: POST '/{domain}' SEND_ONE READ_ONE 201: Create

-----------
Execution
-----------

OP1

-----------
Params
-----------

no_equals_sign
`
	_, err := Parse("test", content)
	if err == nil {
		t.Fatal("expected error for invalid param mapping")
	}
	if !strings.Contains(err.Error(), "invalid param mapping") {
		t.Errorf("expected 'invalid param mapping' error, got: %v", err)
	}
}

func TestParseAllProfiles(t *testing.T) {
	files, err := filepath.Glob("../../profiles/*.profile")
	if err != nil {
		t.Fatalf("globbing profiles: %v", err)
	}

	if len(files) < 2 {
		t.Fatalf("expected at least 2 profile files, found %d", len(files))
	}

	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".profile")
		if name == "template" {
			continue
		}

		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("reading file: %v", err)
			}

			p, err := Parse(name, string(data))
			if err != nil {
				t.Fatalf("parsing: %v", err)
			}

			if p.Name != name {
				t.Errorf("name = %q, want %q", p.Name, name)
			}
			if len(p.Operations) == 0 {
				t.Error("no operations defined")
			}
			if len(p.ExecutionOrder) == 0 {
				t.Error("no execution order defined")
			}

			// Verify all execution order entries reference defined operations
			opNames := make(map[string]bool)
			for _, op := range p.Operations {
				opNames[op.Name] = true
			}
			for _, ref := range p.ExecutionOrder {
				if !opNames[ref] {
					t.Errorf("execution order references undefined operation %q", ref)
				}
			}
		})
	}
}
