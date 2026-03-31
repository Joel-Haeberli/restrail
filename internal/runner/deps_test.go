package runner

import (
	"restrail/internal/openapi"
	"testing"
)

func TestExtractFKStem(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"customerId", "customer"},
		{"customer_id", "customer"},
		{"customerID", "customer"},
		{"customer_ID", "customer"},
		{"organizationId", "organization"},
		{"school_year_id", "school_year"},
		{"postal_code_no", "postal_code"},
		{"postalCodeNo", "postalCode"},
		{"postal_code_NO", "postal_code"},
		{"order_num", "order"},
		{"orderNum", "order"},
		{"invoiceNumber", "invoice"},
		{"invoice_number", "invoice"},
		{"name", ""},
		{"id", ""},
		{"Id", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractFKStem(tt.input)
		if got != tt.want {
			t.Errorf("extractFKStem(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMatchFKToDomain(t *testing.T) {
	domains := map[string]string{
		"customers":    "customers",
		"orders":       "orders",
		"categories":   "categories",
		"school-years": "school-years",
		"users":        "users",
		"postal-codes": "postal-codes",
	}

	tests := []struct {
		field    string
		wantDom  string
		wantOK   bool
	}{
		{"customerId", "customers", true},
		{"customer_id", "customers", true},
		{"orderId", "orders", true},
		{"categoryId", "categories", true},
		{"categoryID", "categories", true},
		{"schoolYearId", "school-years", true},  // camelCase -> kebab domain via normalized match
		{"school_year_id", "school-years", true}, // snake_case -> kebab domain
		{"userId", "users", true},
		{"postal_code_no", "postal-codes", true},  // _no suffix -> kebab domain
		{"postalCodeNo", "postal-codes", true},     // camelCase _No suffix
		{"orderNum", "orders", true},               // Num suffix
		{"name", "", false},
		{"age", "", false},
		{"unknownEntityId", "", false},
	}
	for _, tt := range tests {
		dom, ok := matchFKToDomain(tt.field, domains)
		if ok != tt.wantOK || dom != tt.wantDom {
			t.Errorf("matchFKToDomain(%q) = (%q, %v), want (%q, %v)", tt.field, dom, ok, tt.wantDom, tt.wantOK)
		}
	}
}

func TestTopoSort_NoDeps(t *testing.T) {
	g := &DependencyGraph{
		hardEdges: make(map[string][]string),
		allEdges:  make(map[string][]string),
	}
	names := []string{"a", "b", "c"}
	ordered, circular := g.TopoSort(names)
	if len(circular) != 0 {
		t.Errorf("expected no circular, got %v", circular)
	}
	if len(ordered) != 3 {
		t.Errorf("expected 3 ordered, got %d", len(ordered))
	}
}

func TestTopoSort_LinearChain(t *testing.T) {
	// c -> b -> a (a depends on b, b depends on c)
	g := &DependencyGraph{
		hardEdges: map[string][]string{
			"a": {"b"},
			"b": {"c"},
		},
		allEdges: map[string][]string{
			"a": {"b"},
			"b": {"c"},
		},
	}
	names := []string{"a", "b", "c"}
	ordered, circular := g.TopoSort(names)
	if len(circular) != 0 {
		t.Errorf("expected no circular, got %v", circular)
	}
	// c must come before b, b before a
	idx := make(map[string]int)
	for i, n := range ordered {
		idx[n] = i
	}
	if idx["c"] > idx["b"] {
		t.Errorf("c should come before b: %v", ordered)
	}
	if idx["b"] > idx["a"] {
		t.Errorf("b should come before a: %v", ordered)
	}
}

func TestTopoSort_Diamond(t *testing.T) {
	// d is depended on by both b and c; a depends on both b and c
	g := &DependencyGraph{
		hardEdges: map[string][]string{
			"a": {"b", "c"},
			"b": {"d"},
			"c": {"d"},
		},
		allEdges: map[string][]string{
			"a": {"b", "c"},
			"b": {"d"},
			"c": {"d"},
		},
	}
	names := []string{"a", "b", "c", "d"}
	ordered, circular := g.TopoSort(names)
	if len(circular) != 0 {
		t.Errorf("expected no circular, got %v", circular)
	}
	idx := make(map[string]int)
	for i, n := range ordered {
		idx[n] = i
	}
	if idx["d"] > idx["b"] || idx["d"] > idx["c"] {
		t.Errorf("d should come before b and c: %v", ordered)
	}
	if idx["b"] > idx["a"] || idx["c"] > idx["a"] {
		t.Errorf("b and c should come before a: %v", ordered)
	}
}

func TestTopoSort_Cycle(t *testing.T) {
	// a depends on b, b depends on a
	g := &DependencyGraph{
		hardEdges: map[string][]string{
			"a": {"b"},
			"b": {"a"},
		},
		allEdges: map[string][]string{
			"a": {"b"},
			"b": {"a"},
		},
	}
	names := []string{"a", "b", "c"}
	ordered, circular := g.TopoSort(names)
	if len(ordered) != 3 {
		t.Errorf("expected 3 ordered (including circular), got %d: %v", len(ordered), ordered)
	}
	if !circular["a"] || !circular["b"] {
		t.Errorf("expected a and b to be circular, got %v", circular)
	}
	if circular["c"] {
		t.Errorf("c should not be circular")
	}
}

func TestTopoSort_OptionalEdgesOrdering(t *testing.T) {
	// a depends on b (optional only, not in hardEdges)
	// b has no deps. Expected: b before a.
	g := &DependencyGraph{
		hardEdges: map[string][]string{},
		allEdges: map[string][]string{
			"a": {"b"},
		},
	}
	names := []string{"a", "b", "c"}
	ordered, circular := g.TopoSort(names)
	if len(circular) != 0 {
		t.Errorf("expected no circular, got %v", circular)
	}
	if len(ordered) != 3 {
		t.Errorf("expected 3 ordered, got %d: %v", len(ordered), ordered)
	}
	idx := make(map[string]int)
	for i, n := range ordered {
		idx[n] = i
	}
	if idx["b"] > idx["a"] {
		t.Errorf("b should come before a: %v", ordered)
	}
}

func TestTopoSort_OptionalEdgeBreaksCycle(t *testing.T) {
	// a->b (hard), b->a (optional). The optional b->a edge should be
	// broken, resulting in: b first, then a. No circulars.
	g := &DependencyGraph{
		hardEdges: map[string][]string{
			"a": {"b"},
		},
		allEdges: map[string][]string{
			"a": {"b"},
			"b": {"a"},
		},
	}
	names := []string{"a", "b"}
	ordered, circular := g.TopoSort(names)
	if len(circular) != 0 {
		t.Errorf("expected no circular (optional edge should break cycle), got %v", circular)
	}
	if len(ordered) != 2 {
		t.Errorf("expected 2 ordered, got %d: %v", len(ordered), ordered)
	}
	idx := make(map[string]int)
	for i, n := range ordered {
		idx[n] = i
	}
	if idx["b"] > idx["a"] {
		t.Errorf("b should come before a: %v", ordered)
	}
}

func TestTopoSort_MixedOptionalAndHard(t *testing.T) {
	// Real-world scenario: students->school-classes (hard), school-classes->students (hard) = cycle.
	// rooms->buildings (hard), buildings->locations (optional).
	// Expected: locations before buildings (optional respected), rooms after buildings,
	// students+school-classes marked circular.
	g := &DependencyGraph{
		hardEdges: map[string][]string{
			"students":       {"school-classes"},
			"school-classes": {"students"},
			"rooms":          {"buildings"},
		},
		allEdges: map[string][]string{
			"students":       {"school-classes"},
			"school-classes": {"students"},
			"rooms":          {"buildings"},
			"buildings":      {"locations"},
		},
	}
	names := []string{"students", "school-classes", "rooms", "buildings", "locations"}
	ordered, circular := g.TopoSort(names)

	if !circular["students"] || !circular["school-classes"] {
		t.Errorf("expected students and school-classes circular, got %v", circular)
	}
	if circular["rooms"] || circular["buildings"] || circular["locations"] {
		t.Errorf("rooms, buildings, locations should not be circular, got %v", circular)
	}

	idx := make(map[string]int)
	for i, n := range ordered {
		idx[n] = i
	}
	if idx["locations"] > idx["buildings"] {
		t.Errorf("locations should come before buildings: %v", ordered)
	}
	if idx["buildings"] > idx["rooms"] {
		t.Errorf("buildings should come before rooms: %v", ordered)
	}
}

func TestDependenciesForDomainExcluding(t *testing.T) {
	g := &DependencyGraph{
		Dependencies: []Dependency{
			{FromDomain: "a", ToDomain: "b", FieldName: "bId"},
			{FromDomain: "a", ToDomain: "c", FieldName: "cRef"},
			{FromDomain: "a", ToDomain: "d", FieldName: "dRef"},
			{FromDomain: "b", ToDomain: "c", FieldName: "cId"},
		},
	}

	exclude := map[string]bool{"c": true}
	deps := g.DependenciesForDomainExcluding("a", exclude)
	if len(deps) != 2 {
		t.Errorf("expected 2 deps (excluding c), got %d", len(deps))
	}
	for _, dep := range deps {
		if dep.ToDomain == "c" {
			t.Errorf("should not include excluded domain c")
		}
	}

	// Nil exclude = include all
	deps = g.DependenciesForDomainExcluding("a", nil)
	if len(deps) != 3 {
		t.Errorf("expected 3 deps with nil exclude, got %d", len(deps))
	}
}

func TestCamelToKebab(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"schoolClass", "school-class"},
		{"currentSchoolClass", "current-school-class"},
		{"teacher", "teacher"},
		{"postalCode", "postal-code"},
		{"postalCodes", "postal-codes"},
		{"building", "building"},
		{"pastSchoolClasses", "past-school-classes"},
		{"", ""},
	}
	for _, tt := range tests {
		got := camelToKebab(tt.input)
		if got != tt.want {
			t.Errorf("camelToKebab(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsEntityRef(t *testing.T) {
	tests := []struct {
		name    string
		schema  *openapi.Schema
		wantID  string
		wantOK  bool
	}{
		{
			"EntityRef with id",
			&openapi.Schema{
				Type:       "object",
				Properties: map[string]*openapi.Schema{"id": {Type: "string"}},
				Required:   []string{"id"},
			},
			"id", true,
		},
		{
			"PostalCodeRef with number",
			&openapi.Schema{
				Type:       "object",
				Properties: map[string]*openapi.Schema{"number": {Type: "integer"}},
				Required:   []string{"number"},
			},
			"number", true,
		},
		{
			"nil schema",
			nil,
			"", false,
		},
		{
			"regular object with many properties",
			&openapi.Schema{
				Type: "object",
				Properties: map[string]*openapi.Schema{
					"name": {Type: "string"},
					"age":  {Type: "integer"},
					"city": {Type: "string"},
					"zip":  {Type: "string"},
				},
			},
			"", false,
		},
		{
			"string type not object",
			&openapi.Schema{Type: "string"},
			"", false,
		},
	}
	for _, tt := range tests {
		gotID, gotOK := isEntityRef(tt.schema)
		if gotID != tt.wantID || gotOK != tt.wantOK {
			t.Errorf("isEntityRef(%s) = (%q, %v), want (%q, %v)", tt.name, gotID, gotOK, tt.wantID, tt.wantOK)
		}
	}
}

func TestMatchEntityRefToDomain(t *testing.T) {
	domains := map[string]string{
		"buildings":      "buildings",
		"teachers":       "teachers",
		"students":       "students",
		"school-classes": "school-classes",
		"school-years":   "school-years",
		"postal-codes":   "postal-codes",
		"rooms":          "rooms",
		"addresses":      "addresses",
		"subjects":       "subjects",
		"exams":          "exams",
		"guardians":      "guardians",
		"locations":      "locations",
	}

	tests := []struct {
		field   string
		wantDom string
		wantOK  bool
	}{
		{"building", "buildings", true},
		{"teacher", "teachers", true},
		{"student", "students", true},
		{"schoolClass", "school-classes", true},
		{"schoolYear", "school-years", true},
		{"postalCode", "postal-codes", true},
		{"postalCodes", "postal-codes", true},
		{"address", "addresses", true},
		{"subject", "subjects", true},
		{"exam", "exams", true},
		{"guardians", "guardians", true},
		{"students", "students", true},
		{"teachers", "teachers", true},
		{"location", "locations", true},
		// With prefixes
		{"currentSchoolClass", "school-classes", true},
		{"pastSchoolClasses", "school-classes", true},
		// No match
		{"name", "", false},
		{"version", "", false},
		{"grade", "", false},
	}
	for _, tt := range tests {
		dom, ok := matchEntityRefToDomain(tt.field, domains)
		if ok != tt.wantOK || dom != tt.wantDom {
			t.Errorf("matchEntityRefToDomain(%q) = (%q, %v), want (%q, %v)", tt.field, dom, ok, tt.wantDom, tt.wantOK)
		}
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"customers", "customer"},
		{"categories", "category"},
		{"classes", "class"},
		{"users", "user"},
		{"data", "data"},
		{"boxes", "box"},
	}
	for _, tt := range tests {
		got := singularize(tt.input)
		if got != tt.want {
			t.Errorf("singularize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
