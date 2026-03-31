package runner

import (
	"restrail/internal/openapi"
	"restrail/internal/profile"
	"strings"
)

// RefStyle describes how a foreign key reference is represented in the schema.
type RefStyle int

const (
	RefStyleScalar RefStyle = iota // flat FK: field value is the ID directly (e.g., "buildingId": "uuid")
	RefStyleObject                 // entity reference object: field value is {"id": "uuid"}
	RefStyleArray                  // array of entity refs: field value is [{"id": "uuid"}]
)

// Dependency represents a foreign key relationship between two domains.
type Dependency struct {
	FromDomain string
	ToDomain   string
	FieldName  string
	Required   bool
	RefStyle   RefStyle // how the FK is represented in the schema
	RefIDField string   // field name within the reference object (e.g., "id", "number"); empty for scalar
}

// DependencyGraph holds all detected FK relationships and provides ordering.
type DependencyGraph struct {
	Dependencies []Dependency
	hardEdges    map[string][]string // domain -> domains it depends on (required FKs only)
	allEdges     map[string][]string // domain -> domains it depends on (all FKs)
}

// DetectDependencies scans POST request body schemas for foreign key fields
// and maps them to other domains.
func DetectDependencies(
	domains []profile.DomainMatch,
	spec *openapi.Spec,
	prof *profile.Profile,
) *DependencyGraph {
	graph := &DependencyGraph{
		hardEdges: make(map[string][]string),
		allEdges:  make(map[string][]string),
	}

	// Build set of known domain names (lowercase -> original)
	domainSet := make(map[string]string)
	for _, d := range domains {
		domainSet[strings.ToLower(d.Domain)] = d.Domain
	}

	for _, domain := range domains {
		// Find the POST/create operation for this domain
		for opName, mapping := range domain.Mappings {
			op := prof.OperationByName(opName)
			if op == nil || op.Method != "POST" {
				continue
			}
			if op.SendType != profile.SendOne && op.SendType != profile.SendMany {
				continue
			}

			schema := openapi.GetRequestBodySchema(mapping.SpecOp)
			if schema == nil {
				continue
			}

			// Collect all properties including flattened allOf
			props, required := collectProperties(schema)

			for fieldName, propSchema := range props {
				dep, ok := detectFieldDependency(fieldName, propSchema, domain.Domain, required[fieldName], domainSet)
				if !ok {
					continue
				}

				if hasDep(graph.Dependencies, domain.Domain, dep.ToDomain) {
					continue
				}

				graph.Dependencies = append(graph.Dependencies, dep)
				graph.allEdges[domain.Domain] = append(graph.allEdges[domain.Domain], dep.ToDomain)
				if dep.Required {
					graph.hardEdges[domain.Domain] = append(graph.hardEdges[domain.Domain], dep.ToDomain)
				}
			}
		}
	}

	return graph
}

// TopoSort returns domain names in dependency order (dependencies first)
// and identifies domains involved in circular dependencies.
//
// It uses allEdges (all FK relationships) for ordering so that optional
// dependencies are also respected. When cycles are detected, optional
// edges (present in allEdges but not hardEdges) are broken first. Only
// domains in unbreakable hard cycles are marked as circular.
func (g *DependencyGraph) TopoSort(domainNames []string) (ordered []string, circular map[string]bool) {
	circular = make(map[string]bool)

	if len(g.allEdges) == 0 {
		return domainNames, circular
	}

	// Build a hard-edge lookup set for quick "is this edge required?" checks.
	hardSet := make(map[[2]string]bool)
	for from, deps := range g.hardEdges {
		for _, to := range deps {
			hardSet[[2]string{from, to}] = true
		}
	}

	// Kahn's algorithm on allEdges.
	// inDegree[A] = number of domains A depends on (edges FROM A TO its deps).
	inDegree := make(map[string]int)
	for _, name := range domainNames {
		inDegree[name] = 0
	}
	for from, deps := range g.allEdges {
		inDegree[from] += len(deps)
	}

	// Reverse edges: for each edge A->B, record B->A so when B is
	// "processed" we can decrement A's inDegree.
	reverse := make(map[string][]string)
	for from, deps := range g.allEdges {
		for _, to := range deps {
			reverse[to] = append(reverse[to], from)
		}
	}

	// Seed queue with domains that have no dependencies.
	var queue []string
	for _, name := range domainNames {
		if inDegree[name] == 0 {
			queue = append(queue, name)
		}
	}

	// Phase 1: standard Kahn's
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		ordered = append(ordered, node)

		for _, dependent := range reverse[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Phase 2: break cycles by removing optional edges.
	// Remaining nodes (inDegree > 0) are in cycles. Remove optional
	// edges first to try to unblock them.
	if len(ordered) < len(domainNames) {
		// Collect remaining nodes.
		remaining := make(map[string]bool)
		for _, name := range domainNames {
			if inDegree[name] > 0 {
				remaining[name] = true
			}
		}

		// Remove optional edges among remaining nodes.
		for from := range remaining {
			for _, to := range g.allEdges[from] {
				if remaining[to] && !hardSet[[2]string{from, to}] {
					// This is an optional edge in a cycle — break it.
					inDegree[from]--
				}
			}
		}

		// Re-seed queue with unblocked nodes.
		for _, name := range domainNames {
			if remaining[name] && inDegree[name] == 0 {
				queue = append(queue, name)
				remaining[name] = false
			}
		}

		// Continue Kahn's on unblocked nodes.
		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]
			ordered = append(ordered, node)

			for _, dependent := range reverse[node] {
				if !remaining[dependent] {
					continue
				}
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					queue = append(queue, dependent)
					remaining[dependent] = false
				}
			}
		}

		// Any still-remaining nodes are in hard cycles.
		for _, name := range domainNames {
			if remaining[name] {
				circular[name] = true
				ordered = append(ordered, name)
			}
		}
	}

	return ordered, circular
}

// DependenciesForDomain returns all FK dependencies where FromDomain matches.
func (g *DependencyGraph) DependenciesForDomain(domain string) []Dependency {
	var result []Dependency
	for _, dep := range g.Dependencies {
		if dep.FromDomain == domain {
			result = append(result, dep)
		}
	}
	return result
}

// DependenciesForDomainExcluding returns FK dependencies for a domain,
// excluding dependencies whose ToDomain is in the provided set.
func (g *DependencyGraph) DependenciesForDomainExcluding(domain string, exclude map[string]bool) []Dependency {
	var result []Dependency
	for _, dep := range g.Dependencies {
		if dep.FromDomain == domain && !exclude[dep.ToDomain] {
			result = append(result, dep)
		}
	}
	return result
}

// collectProperties gathers top-level properties from a schema,
// flattening allOf but not recursing into nested objects.
func collectProperties(schema *openapi.Schema) (props map[string]*openapi.Schema, required map[string]bool) {
	props = make(map[string]*openapi.Schema)
	required = make(map[string]bool)

	var collect func(s *openapi.Schema)
	collect = func(s *openapi.Schema) {
		if s == nil {
			return
		}
		for name, prop := range s.Properties {
			props[name] = prop
		}
		for _, r := range s.Required {
			required[r] = true
		}
		for _, sub := range s.AllOf {
			collect(sub)
		}
	}
	collect(schema)
	return
}

// matchFKToDomain tries to match a scalar FK field name to a domain.
// It strips FK suffixes (e.g., "Id", "_id") then matches the stem to a domain.
func matchFKToDomain(fieldName string, domainSet map[string]string) (string, bool) {
	stem := extractFKStem(fieldName)
	if stem == "" {
		return "", false
	}
	return matchNameToDomain(strings.ToLower(stem), domainSet)
}

// extractFKStem strips common FK suffixes from a field name and returns the entity stem.
// Returns "" if the field doesn't look like a FK.
func extractFKStem(name string) string {
	// Try suffixes in order of specificity (longest first within each group).
	// Covers: _id/_ID/Id/ID, _no/_NO/No/NO, _num/_NUM/Num/NUM, _number/_NUMBER/Number
	for _, suffix := range []string{
		"_number", "_NUMBER", "Number",
		"_id", "_ID", "Id", "ID",
		"_no", "_NO", "No", "NO",
		"_num", "_NUM", "Num", "NUM",
	} {
		if strings.HasSuffix(name, suffix) && len(name) > len(suffix) {
			stem := name[:len(name)-len(suffix)]
			// For camelCase: "customerId" -> stem "customer"
			// For snake_case: "customer_id" -> stem "customer"
			stem = strings.TrimRight(stem, "_")
			if stem != "" {
				return stem
			}
		}
	}
	return ""
}

// singularize does basic English singularization.
func singularize(s string) string {
	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "ses") || strings.HasSuffix(s, "xes") || strings.HasSuffix(s, "zes") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "es") && len(s) > 2 {
		// Only strip "es" for words ending in s, sh, ch, x, z
		base := s[:len(s)-2]
		if strings.HasSuffix(base, "s") || strings.HasSuffix(base, "sh") ||
			strings.HasSuffix(base, "ch") || strings.HasSuffix(base, "x") ||
			strings.HasSuffix(base, "z") {
			return base
		}
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") && len(s) > 1 {
		return s[:len(s)-1]
	}
	return s
}

// detectFieldDependency checks a single schema property for a FK relationship.
// It tries scalar FK suffixes first, then entity-reference objects, then arrays of entity refs.
func detectFieldDependency(fieldName string, propSchema *openapi.Schema, fromDomain string, isRequired bool, domainSet map[string]string) (Dependency, bool) {
	// Strategy 1: Scalar FK suffix (e.g., "buildingId", "category_id")
	if target, ok := matchFKToDomain(fieldName, domainSet); ok && target != fromDomain {
		return Dependency{
			FromDomain: fromDomain,
			ToDomain:   target,
			FieldName:  fieldName,
			Required:   isRequired,
			RefStyle:   RefStyleScalar,
		}, true
	}

	// Strategy 2: Single entity-reference object (e.g., "building": {"id": "uuid"})
	if refIDField, isRef := isEntityRef(propSchema); isRef {
		if target, ok := matchEntityRefToDomain(fieldName, domainSet); ok && target != fromDomain {
			return Dependency{
				FromDomain: fromDomain,
				ToDomain:   target,
				FieldName:  fieldName,
				Required:   isRequired,
				RefStyle:   RefStyleObject,
				RefIDField: refIDField,
			}, true
		}
	}

	// Strategy 3: Array of entity-reference objects (e.g., "teachers": [{"id": "uuid"}])
	if propSchema != nil && propSchema.Type == "array" && propSchema.Items != nil {
		if refIDField, isRef := isEntityRef(propSchema.Items); isRef {
			if target, ok := matchEntityRefToDomain(fieldName, domainSet); ok && target != fromDomain {
				return Dependency{
					FromDomain: fromDomain,
					ToDomain:   target,
					FieldName:  fieldName,
					Required:   isRequired,
					RefStyle:   RefStyleArray,
					RefIDField: refIDField,
				}, true
			}
		}
	}

	return Dependency{}, false
}

// isEntityRef checks if a schema looks like an entity reference object
// (a small object with an ID-like field). Returns the ID field name.
func isEntityRef(schema *openapi.Schema) (string, bool) {
	if schema == nil {
		return "", false
	}
	// Must be an object (explicit or implicit via properties)
	if schema.Type != "object" && schema.Type != "" {
		return "", false
	}
	if len(schema.Properties) == 0 || len(schema.Properties) > 3 {
		return "", false // too many properties to be a simple ref
	}

	// Check for common ID field names
	for _, name := range []string{"id", "Id", "ID", "_id"} {
		if _, exists := schema.Properties[name]; exists {
			return name, true
		}
	}

	// If there's exactly one required field, treat it as the ref identifier
	// (handles cases like PostalCodeRef with "number" as the ID)
	if len(schema.Required) == 1 {
		if _, exists := schema.Properties[schema.Required[0]]; exists {
			return schema.Required[0], true
		}
	}

	return "", false
}

// matchEntityRefToDomain matches a field name directly to a domain.
// Unlike matchFKToDomain (which strips FK suffixes), this matches the field name
// as-is, handling camelCase→kebab conversion and singular/plural variations.
// E.g., "schoolClass" → "school-classes", "teacher" → "teachers".
func matchEntityRefToDomain(fieldName string, domainSet map[string]string) (string, bool) {
	// Convert camelCase to kebab-case for matching
	kebab := camelToKebab(fieldName)

	if target, ok := matchNameToDomain(kebab, domainSet); ok {
		return target, true
	}

	// Try stripping common prefixes (for "currentSchoolClass" → "school-class")
	for _, prefix := range []string{
		"current-", "past-", "new-", "old-", "parent-",
		"source-", "target-", "from-", "to-", "default-",
	} {
		if strings.HasPrefix(kebab, prefix) {
			stripped := kebab[len(prefix):]
			if stripped != "" {
				if target, ok := matchNameToDomain(stripped, domainSet); ok {
					return target, true
				}
			}
		}
	}

	return "", false
}

// matchNameToDomain tries to match a normalized name (lowercase/kebab) to a domain
// using singular/plural variations and hyphen/underscore normalization.
func matchNameToDomain(name string, domainSet map[string]string) (string, bool) {
	name = strings.ToLower(name)

	// Exact match
	if orig, ok := domainSet[name]; ok {
		return orig, true
	}

	// Pluralize: name + "s"
	if orig, ok := domainSet[name+"s"]; ok {
		return orig, true
	}

	// Pluralize: name + "es"
	if orig, ok := domainSet[name+"es"]; ok {
		return orig, true
	}

	// Pluralize: name ending in "y" → "ies"
	if strings.HasSuffix(name, "y") {
		if orig, ok := domainSet[name[:len(name)-1]+"ies"]; ok {
			return orig, true
		}
	}

	// Singularize domain names and compare
	for domLower, orig := range domainSet {
		if singularize(domLower) == name {
			return orig, true
		}
	}

	// Normalize: strip hyphens/underscores for comparison
	normName := strings.ReplaceAll(strings.ReplaceAll(name, "-", ""), "_", "")
	for domLower, orig := range domainSet {
		normDom := strings.ReplaceAll(strings.ReplaceAll(domLower, "-", ""), "_", "")
		if normDom == normName || normDom == normName+"s" || normDom == normName+"es" {
			return orig, true
		}
		if singularize(normDom) == normName {
			return orig, true
		}
	}

	return "", false
}

// camelToKebab converts a camelCase string to kebab-case.
// E.g., "schoolClass" → "school-class", "currentSchoolClass" → "current-school-class"
func camelToKebab(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result = append(result, '-')
			}
			result = append(result, c|0x20) // toLower
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

func hasDep(deps []Dependency, from, to string) bool {
	for _, d := range deps {
		if d.FromDomain == from && d.ToDomain == to {
			return true
		}
	}
	return false
}
