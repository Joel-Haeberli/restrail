package generator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"restrail/internal/config"
	"restrail/internal/openapi"
	"restrail/internal/plan"
	"restrail/internal/profile"
	"restrail/internal/runner"
	"restrail/internal/testdata"
	"restrail/internal/tmpl"
	"strings"
)

// GenerateResult holds the summary of what was generated.
type GenerateResult struct {
	OutputDir     string
	RequestFiles  []string
	PlanFile      string
	RunConfigFile string
}

// Generate runs the init phase: parses spec+profile, detects deps, generates .request files,
// execution.plan, and a template run config.
func Generate(spec *openapi.Spec, prof *profile.Profile, cfg config.InitConfig) (*GenerateResult, error) {
	outputDir := cfg.ResolvedOutputDir()

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Match profile against spec
	domains := profile.MatchProfile(spec, prof)
	if len(domains) == 0 {
		return nil, fmt.Errorf("no domains matched profile %q", prof.Name)
	}

	// Detect dependencies and sort
	depGraph := runner.DetectDependencies(domains, spec, prof)
	domainMap := make(map[string]profile.DomainMatch)
	var domainNames []string
	for _, d := range domains {
		domainMap[d.Domain] = d
		domainNames = append(domainNames, d.Domain)
	}

	orderedNames, circularDomains := depGraph.TopoSort(domainNames)

	factory := testdata.NewRandomFactory(cfg.OptimisticLocking)

	result := &GenerateResult{
		OutputDir: outputDir,
	}

	// Build phases for the execution plan
	var phase1Steps []plan.Step
	var phase2Steps []plan.Step

	// Phase 1: Create and verify (dependency order)
	for _, name := range orderedNames {
		domain := domainMap[name]
		steps, err := generateDomainRequests(outputDir, prof, domain, depGraph, circularDomains, factory)
		if err != nil {
			return nil, fmt.Errorf("generating requests for domain %s: %w", name, err)
		}
		for _, s := range steps {
			result.RequestFiles = append(result.RequestFiles, s.Filename)
			if isDeleteStep(s.Filename) {
				phase2Steps = append([]plan.Step{s}, phase2Steps...) // prepend for reverse order
			} else {
				phase1Steps = append(phase1Steps, s)
			}
		}
	}

	// Write execution plan
	var depLines []string
	for _, dep := range depGraph.Dependencies {
		style := "scalar"
		switch dep.RefStyle {
		case runner.RefStyleObject:
			style = "object"
		case runner.RefStyleArray:
			style = "array"
		}
		depLines = append(depLines, fmt.Sprintf("%s.%s -> %s (%s)", dep.FromDomain, dep.FieldName, dep.ToDomain, style))
	}

	phases := []plan.Phase{
		{Name: "Phase 1: Create and verify (dependency order)", Steps: phase1Steps},
	}
	if len(phase2Steps) > 0 {
		phases = append(phases, plan.Phase{Name: "Phase 2: Cleanup (reverse dependency order)", Steps: phase2Steps})
	}

	if err := plan.Write(outputDir, prof.Name, phases, orderedNames, depLines, prof.ParamMappings); err != nil {
		return nil, fmt.Errorf("writing execution plan: %w", err)
	}
	result.PlanFile = filepath.Join(outputDir, "execution.plan")

	// Write template run config
	runCfg := buildRunConfigTemplate(spec, outputDir, cfg)
	runConfigPath, err := writeRunConfig(outputDir, runCfg)
	if err != nil {
		return nil, fmt.Errorf("writing run config: %w", err)
	}
	result.RunConfigFile = runConfigPath

	return result, nil
}

func generateDomainRequests(
	dir string,
	prof *profile.Profile,
	domain profile.DomainMatch,
	depGraph *runner.DependencyGraph,
	circularDomains map[string]bool,
	factory testdata.Factory,
) ([]plan.Step, error) {
	var steps []plan.Step

	// Track GET operations for disambiguation
	getCount := 0
	for _, opName := range prof.ExecutionOrder {
		op := prof.OperationByName(opName)
		if op != nil && op.Method == "GET" {
			if _, exists := domain.Mappings[opName]; exists {
				getCount++
			}
		}
	}

	for _, opName := range prof.ExecutionOrder {
		mapping, exists := domain.Mappings[opName]
		op := prof.OperationByName(opName)
		if op == nil || !exists {
			continue
		}

		// Determine filename suffix for disambiguation
		crudSuffix := ""
		if op.Method == "GET" && getCount > 1 {
			if strings.Contains(op.Pattern, "{id}") {
				crudSuffix = "BY_ID"
			} else {
				crudSuffix = "LIST"
			}
		}

		filename := RequestFilename(domain.Domain, op.Method, crudSuffix)

		// Build headers
		headers := map[string]string{
			"Accept":        "application/json",
			"Authorization": "__AUTH__",
		}

		// Generate body for POST/PUT/PATCH
		var body map[string]interface{}
		if op.SendType == profile.SendOne || op.SendType == profile.SendMany {
			headers["Content-Type"] = "application/json"

			schema := openapi.GetRequestBodySchema(mapping.SpecOp)
			var err error
			body, err = factory.Generate(schema)
			if err != nil {
				return nil, fmt.Errorf("generating test data for %s %s: %w", op.Method, mapping.SpecPath, err)
			}

			// Remove optional fields that are not FK dependencies.
			// Optional fields with factory-generated values (e.g. "test_j2pkueho") cause
			// backend constraint errors at runtime. Required fields stay; optional FK dep
			// fields are added with sentinels by the loop below.
			schemaRequired := collectSchemaRequired(schema)
			for fieldName := range body {
				if !schemaRequired[fieldName] {
					delete(body, fieldName)
				}
			}

			// Replace FK fields with sentinel values (converted to template expressions by WriteRequest).
			// Skip circular dependencies entirely; use fk_optional for optional deps.
			deps := depGraph.DependenciesForDomainExcluding(domain.Domain, circularDomains)
			for _, dep := range deps {
				var sentinel string
				if dep.Required {
					sentinel = tmpl.FKSentinel(dep.ToDomain)
				} else {
					sentinel = tmpl.FKOptionalSentinel(dep.ToDomain)
				}
				switch dep.RefStyle {
				case runner.RefStyleScalar:
					body[dep.FieldName] = sentinel
				case runner.RefStyleObject:
					refIDField := dep.RefIDField
					if refIDField == "" {
						refIDField = "id"
					}
					body[dep.FieldName] = map[string]interface{}{refIDField: sentinel}
				case runner.RefStyleArray:
					refIDField := dep.RefIDField
					if refIDField == "" {
						refIDField = "id"
					}
					body[dep.FieldName] = []interface{}{map[string]interface{}{refIDField: sentinel}}
				}
			}
		}

		if err := WriteRequest(dir, filename, op.Method, mapping.SpecPath, headers, body); err != nil {
			return nil, fmt.Errorf("writing request file %s: %w", filename, err)
		}

		// Build plan step with annotations
		var annotations []string
		if op.Method == "POST" && op.ReadType == profile.ReadOne {
			annotations = append(annotations, "EXTRACT_ID")
		}

		// Add INJECT_FK annotations for operations that have FK dependencies (excluding circular)
		if op.SendType == profile.SendOne || op.SendType == profile.SendMany {
			deps := depGraph.DependenciesForDomainExcluding(domain.Domain, circularDomains)
			for _, dep := range deps {
				annotations = append(annotations, fmt.Sprintf("INJECT_FK %s=%s", dep.FieldName, dep.ToDomain))
			}
		}

		steps = append(steps, plan.Step{
			Filename:    filename,
			Annotations: annotations,
		})
	}

	return steps, nil
}

// collectSchemaRequired returns the set of required field names declared in a
// schema, recursively flattening allOf fragments.
func collectSchemaRequired(schema *openapi.Schema) map[string]bool {
	required := make(map[string]bool)
	if schema == nil {
		return required
	}
	var collect func(s *openapi.Schema)
	collect = func(s *openapi.Schema) {
		for _, r := range s.Required {
			required[r] = true
		}
		for _, sub := range s.AllOf {
			collect(sub)
		}
	}
	collect(schema)
	return required
}

func isDeleteStep(filename string) bool {
	return strings.Contains(filename, "_DELETE")
}

func buildRunConfigTemplate(spec *openapi.Spec, testDir string, initCfg config.InitConfig) config.RunConfig {
	rc := config.RunConfig{
		TestDir:           testDir,
		OptimisticLocking: initCfg.OptimisticLocking,
		SpecPath:          initCfg.SpecPath,
		AuthType:          "none",
	}

	// Detect auth type from spec
	for _, scheme := range spec.Components.SecuritySchemes {
		switch scheme.Type {
		case "oauth2":
			rc.AuthType = "oauth2"
			if scheme.Flows != nil {
				if scheme.Flows.ClientCredentials != nil {
					rc.TokenURL = scheme.Flows.ClientCredentials.TokenURL
					rc.Scopes = scopeKeys(scheme.Flows.ClientCredentials.Scopes)
				} else if scheme.Flows.Password != nil {
					rc.TokenURL = scheme.Flows.Password.TokenURL
					rc.Scopes = scopeKeys(scheme.Flows.Password.Scopes)
				} else if scheme.Flows.AuthorizationCode != nil {
					rc.TokenURL = scheme.Flows.AuthorizationCode.TokenURL
					rc.Scopes = scopeKeys(scheme.Flows.AuthorizationCode.Scopes)
				}
			}
		case "openIdConnect":
			rc.AuthType = "oidc"
			rc.TokenURL = scheme.OpenIDConnectURL
		case "http":
			if strings.EqualFold(scheme.Scheme, "basic") {
				rc.AuthType = "basic"
			} else if strings.EqualFold(scheme.Scheme, "bearer") {
				rc.AuthType = "bearer"
			}
		}
		// Use the first scheme found
		break
	}

	return rc
}

func scopeKeys(scopes map[string]string) []string {
	if len(scopes) == 0 {
		return nil
	}
	var keys []string
	for k := range scopes {
		keys = append(keys, k)
	}
	return keys
}

func writeRunConfig(dir string, cfg config.RunConfig) (string, error) {
	newData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling run config: %w", err)
	}
	newData = append(newData, '\n')

	target := filepath.Join(dir, "restrail_run_config.json")

	// If the file already exists, check if it was edited by the user.
	// Compare hashes: if they differ, the user has modified it — write
	// to restrail_run_config_edited.json instead to preserve their changes.
	if existing, err := os.ReadFile(target); err == nil {
		existingHash := sha256.Sum256(existing)
		newHash := sha256.Sum256(newData)
		if existingHash != newHash {
			edited := filepath.Join(dir, "restrail_run_config_edited.json")
			return edited, os.WriteFile(edited, newData, 0644)
		}
		// Hashes match — file unchanged by user, safe to overwrite
	}

	return target, os.WriteFile(target, newData, 0644)
}
