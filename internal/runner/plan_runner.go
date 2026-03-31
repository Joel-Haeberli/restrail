package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"restrail/internal/auth"
	"restrail/internal/plan"
	"restrail/internal/testdata"
	"restrail/internal/tmpl"
	"strings"
	"time"
)

// PlanRunner executes tests from generated .request files and an execution plan.
type PlanRunner struct {
	BaseURL           string
	Auth              auth.Authenticator
	TestDir           string
	Client            *http.Client
	OptimisticLocking bool
	Verbose           bool
	BlockedDomains    map[string]struct{}
	IDRegistry        map[string]interface{}            // domain -> created resource ID
	IDFieldRegistry   map[string]string                 // domain -> ID field name
	VersionRegistry   map[string]map[string]interface{} // domain -> version fields
	ParamMappings     map[string]string                 // paramName -> domain ("$" = current domain)
	createdResources  []CreatedResource
}

func NewPlanRunner(baseURL string, authenticator auth.Authenticator, testDir string, optimisticLocking bool, verbose bool, blockedDomains []string) *PlanRunner {
	blocked := make(map[string]struct{}, len(blockedDomains))
	for _, d := range blockedDomains {
		blocked[d] = struct{}{}
	}
	return &PlanRunner{
		BaseURL:           baseURL,
		Auth:              authenticator,
		TestDir:           testDir,
		OptimisticLocking: optimisticLocking,
		Verbose:           verbose,
		BlockedDomains:    blocked,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
		IDRegistry:      make(map[string]interface{}),
		IDFieldRegistry: make(map[string]string),
		VersionRegistry: make(map[string]map[string]interface{}),
	}
}

func (r *PlanRunner) logv(format string, args ...interface{}) {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "[VERBOSE] "+format+"\n", args...)
	}
}

// RunPlan executes the full plan and returns a RunResult.
func (r *PlanRunner) RunPlan(p *plan.ExecutionPlan) *RunResult {
	result := &RunResult{
		Timestamp: time.Now(),
		BaseURL:   r.BaseURL,
		AuthType:  r.Auth.Name(),
	}

	// Load param mappings from execution plan
	if p.ParamMappings != nil {
		r.ParamMappings = p.ParamMappings
		r.logv("Param mappings loaded: %v", r.ParamMappings)
	}

	// Group steps by domain for DomainResult reporting
	domainResults := make(map[string]*DomainResult)
	var domainOrder []string

	r.logv("Starting plan execution: %d total steps, base_url=%s, auth=%s", len(p.AllSteps()), r.BaseURL, r.Auth.Name())
	r.logv("Profile: %s, Domains: %v", p.Profile, p.Domains)
	r.logv("Optimistic locking: %v", r.OptimisticLocking)
	r.logv("Test directory: %s", r.TestDir)

	for i, step := range p.AllSteps() {
		domain := domainFromFilename(step.Filename)
		r.logv("--- Step %d/%d: %s (domain: %s) ---", i+1, len(p.AllSteps()), step.Filename, domain)
		if len(step.Annotations) > 0 {
			r.logv("  Annotations: %v", step.Annotations)
		}

		if _, exists := domainResults[domain]; !exists {
			domainResults[domain] = &DomainResult{
				Domain:  domain,
				Success: true,
			}
			domainOrder = append(domainOrder, domain)
		}

		if _, isBlocked := r.BlockedDomains[domain]; isBlocked {
			r.logv("  Skipping step (domain %q is blocked)", domain)
			domainResults[domain].Operations = append(domainResults[domain].Operations, OperationResult{
				OperationName: step.Filename,
				Skipped:       true,
				SkipReason:    "domain blocked",
			})
			continue
		}

		opResult := r.executeStep(step)
		dr := domainResults[domain]
		dr.Operations = append(dr.Operations, opResult)
		if !opResult.Success && !opResult.Skipped {
			dr.Success = false
			r.logv("  Result: FAIL - %s", opResult.Error)
		} else if opResult.Skipped {
			r.logv("  Result: SKIPPED - %s", opResult.SkipReason)
		} else {
			r.logv("  Result: PASS (status %d, %.0fms)", opResult.ActualStatus, float64(opResult.Duration))
		}

		// Extract ID if annotated
		if step.HasAnnotation("EXTRACT_ID") && opResult.Success && opResult.ResponseBody != nil {
			rawID, field := extractResourceID(opResult.ResponseBody, "")
			if rawID != nil {
				r.IDRegistry[domain] = rawID
				r.IDFieldRegistry[domain] = field
				r.createdResources = append(r.createdResources, CreatedResource{
					Domain:  domain,
					ID:      fmt.Sprintf("%v", rawID),
					IDField: field,
				})
				r.logv("  IDRegistry WRITE: domain=%q -> id=%v (field=%q)", domain, rawID, field)
			} else {
				r.logv("  IDRegistry WRITE: domain=%q -> no ID found in response", domain)
			}
		}

		// Extract version fields
		if r.OptimisticLocking && opResult.Success && opResult.ResponseBody != nil {
			if vf := extractVersionFields(opResult.ResponseBody); len(vf) > 0 {
				r.VersionRegistry[domain] = vf
				r.logv("  VersionRegistry WRITE: domain=%q -> %v", domain, vf)
			}
		}
	}

	// Build result
	for _, name := range domainOrder {
		result.Domains = append(result.Domains, *domainResults[name])
	}

	result.CreatedResources = r.createdResources
	r.computeSummary(result)
	return result
}

func (r *PlanRunner) executeStep(step plan.Step) OperationResult {
	domain := domainFromFilename(step.Filename)
	reqPath := filepath.Join(r.TestDir, step.Filename)

	// Read raw template content
	rawContent, err := os.ReadFile(reqPath)
	if err != nil {
		return OperationResult{
			OperationName: step.Filename,
			Method:        "?",
			Error:         fmt.Sprintf("reading request file: %v", err),
		}
	}
	r.logv("  Template input:\n%s", string(rawContent))

	// Log current registry state before template execution
	if r.Verbose && len(r.IDRegistry) > 0 {
		r.logv("  IDRegistry READ (available for fk/param resolution):")
		for d, id := range r.IDRegistry {
			r.logv("    %s = %v (field: %s)", d, id, r.IDFieldRegistry[d])
		}
	}

	// Execute the template to resolve FK and path param placeholders
	funcMap := tmpl.NewFuncMap(r.IDRegistry, r.makeParamResolver(domain))
	resolved, err := tmpl.Execute(step.Filename, string(rawContent), funcMap)
	if err != nil {
		r.logv("  Template execution FAILED: %v", err)
		return OperationResult{
			OperationName: step.Filename,
			Method:        "?",
			Error:         fmt.Sprintf("executing template: %v", err),
		}
	}
	r.logv("  Template resolved:\n%s", resolved)

	// Parse the resolved content
	parsed, err := parseRequestString(resolved)
	if err != nil {
		return OperationResult{
			OperationName: step.Filename,
			Method:        "?",
			Error:         fmt.Sprintf("parsing resolved request: %v", err),
		}
	}

	result := OperationResult{
		OperationName: step.Filename,
		Method:        parsed.method,
		Path:          parsed.path,
		AuthApplied:   r.Auth.Name(),
	}

	fullURL := r.BaseURL + parsed.path
	result.Path = parsed.path

	// Process body: inject runtime fields (version, ID for PUT/PATCH)
	body := parsed.body
	if body != "" {
		// Remove optional FK fields that were not in the ID registry at template execution time.
		var omitErr error
		body, omitErr = tmpl.RemoveOmitFields(body)
		if omitErr != nil {
			result.Error = fmt.Sprintf("removing omit fields: %v", omitErr)
			return result
		}

		// Inject version fields for PUT/PATCH
		if r.OptimisticLocking && (parsed.method == "PUT" || parsed.method == "PATCH") {
			if vf, ok := r.VersionRegistry[domain]; ok && len(vf) > 0 {
				r.logv("  VersionRegistry READ: domain=%q -> injecting %v", domain, vf)
			}
			body = injectVersionFields(body, r.VersionRegistry[domain])
		}

		// Inject ID for PUT/PATCH
		if parsed.method == "PUT" || parsed.method == "PATCH" {
			if rawID, ok := r.IDRegistry[domain]; ok {
				if idField, ok := r.IDFieldRegistry[domain]; ok {
					r.logv("  IDRegistry READ: injecting %s=%v into %s body", idField, rawID, parsed.method)
					body = injectFieldInBody(body, idField, rawID)
				}
			}
		}

		r.logv("  Final request body:\n%s", body)
	}

	// Build HTTP request
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
		var parsedBody interface{}
		if err := json.Unmarshal([]byte(body), &parsedBody); err == nil {
			result.RequestBody = parsedBody
		}
	}

	r.logv("  HTTP %s %s", parsed.method, fullURL)
	req, err := http.NewRequest(parsed.method, fullURL, bodyReader)
	if err != nil {
		result.Error = fmt.Sprintf("creating request: %v", err)
		return result
	}

	// Set headers from parsed request (except Authorization which we replace)
	for key, val := range parsed.headers {
		if key == "Authorization" {
			continue
		}
		req.Header.Set(key, val)
		r.logv("  Header: %s: %s", key, val)
	}

	// Authenticate
	if err := r.Auth.Authenticate(req); err != nil {
		result.Error = fmt.Sprintf("authentication: %v", err)
		return result
	}
	if authHeader := req.Header.Get("Authorization"); authHeader != "" {
		result.AuthToken = authHeader
		r.logv("  Auth applied: %s (token: %s...)", r.Auth.Name(), truncate(authHeader, 40))
	}

	// Execute
	start := time.Now()
	resp, err := r.Client.Do(req)
	result.Duration = DurationMs(time.Since(start))

	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		r.logv("  HTTP error: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.ActualStatus = resp.StatusCode
	r.logv("  Response: %d %s (%.0fms)", resp.StatusCode, resp.Status, float64(result.Duration))

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("reading response: %v", err)
		return result
	}

	if len(respBody) > 0 {
		r.logv("  Response body (%d bytes):\n%s", len(respBody), truncate(string(respBody), 2000))
		var parsedResp interface{}
		if err := json.Unmarshal(respBody, &parsedResp); err == nil {
			result.ResponseBody = parsedResp
		} else {
			result.ResponseBody = string(respBody)
		}
	} else {
		r.logv("  Response body: (empty)")
	}

	// Determine expected status from filename convention
	expectedStatus := inferExpectedStatus(parsed.method)
	result.ExpectedStatus = expectedStatus

	if resp.StatusCode != expectedStatus {
		if !isAcceptableStatus(parsed.method, resp.StatusCode) {
			result.Error = fmt.Sprintf("expected status %d, got %d", expectedStatus, resp.StatusCode)
			r.logv("  Status mismatch: expected %d, got %d", expectedStatus, resp.StatusCode)
			return result
		}
		r.logv("  Status %d accepted (expected %d, but %d is acceptable for %s)", resp.StatusCode, expectedStatus, resp.StatusCode, parsed.method)
	}

	result.Success = true
	return result
}

// extractVersionFields pulls optimistic-locking fields from a response object.
func extractVersionFields(body interface{}) map[string]interface{} {
	m, ok := body.(map[string]interface{})
	if !ok {
		return nil
	}
	var result map[string]interface{}
	for key, val := range m {
		if testdata.IsVersionField(key) {
			if result == nil {
				result = make(map[string]interface{})
			}
			result[key] = val
		}
	}
	return result
}

// extractResourceID returns (raw value, field name) preserving the original type
// from the API response. idHint is the path parameter name used as a fallback.
func extractResourceID(body interface{}, idHint string) (interface{}, string) {
	m, ok := body.(map[string]interface{})
	if !ok {
		return nil, ""
	}

	for _, field := range []string{"id", "Id", "ID", "_id"} {
		if val, exists := m[field]; exists {
			return val, field
		}
	}

	for key, val := range m {
		lower := strings.ToLower(key)
		if strings.HasSuffix(lower, "id") || strings.HasSuffix(lower, "_id") {
			return val, key
		}
	}

	if idHint != "" {
		if val, exists := m[idHint]; exists {
			return val, idHint
		}
	}

	return nil, ""
}

// makeParamResolver returns a function that resolves all {paramName} placeholders
// in a path using the ID registry and FK-style domain matching.
func (r *PlanRunner) makeParamResolver(currentDomain string) func(string) string {
	return func(rawPath string) string {
		result := rawPath
		for {
			start := strings.Index(result, "{")
			if start == -1 {
				break
			}
			end := strings.Index(result[start:], "}")
			if end == -1 {
				break
			}
			paramName := result[start+1 : start+end]
			id := r.resolveParam(paramName, currentDomain)
			if id == "" {
				// Can't resolve — leave placeholder
				break
			}
			result = result[:start] + id + result[start+end+1:]
		}
		return result
	}
}

// resolveParam tries to find the right ID value for a path parameter.
func (r *PlanRunner) resolveParam(paramName, currentDomain string) string {
	// 1. Check explicit param mappings first
	if r.ParamMappings != nil {
		if target, ok := r.ParamMappings[paramName]; ok {
			if target == "$" {
				target = currentDomain
			}
			if rawID, exists := r.IDRegistry[target]; exists {
				r.logv("    Param {%s} -> domain %q (explicit mapping) -> %v", paramName, target, rawID)
				return fmt.Sprintf("%v", rawID)
			}
			r.logv("    Param {%s} -> domain %q (explicit mapping) but no ID in registry", paramName, target)
			return ""
		}
	}

	// 2. Heuristic FK-style matching
	domainSet := make(map[string]string)
	for domain := range r.IDRegistry {
		domainSet[strings.ToLower(domain)] = domain
	}

	if target, ok := matchFKParamToDomain(paramName, domainSet); ok {
		if rawID, exists := r.IDRegistry[target]; exists {
			r.logv("    Param {%s} -> domain %q (FK match) -> %v", paramName, target, rawID)
			return fmt.Sprintf("%v", rawID)
		}
		r.logv("    Param {%s} -> domain %q matched but no ID in registry", paramName, target)
	}

	// 3. Fall back to the current domain's own ID
	if rawID, ok := r.IDRegistry[currentDomain]; ok {
		r.logv("    Param {%s} -> current domain %q (fallback) -> %v", paramName, currentDomain, rawID)
		return fmt.Sprintf("%v", rawID)
	}

	r.logv("    Param {%s} -> UNRESOLVED (no match, no fallback)", paramName)
	return ""
}

// matchFKParamToDomain matches a path parameter name to a domain using FK-style
// suffix stripping and singular/plural matching.
func matchFKParamToDomain(paramName string, domainSet map[string]string) (string, bool) {
	stem := extractParamStem(paramName)
	if stem == "" {
		lower := strings.ToLower(paramName)
		return matchParamNameToDomain(lower, domainSet)
	}
	return matchParamNameToDomain(strings.ToLower(stem), domainSet)
}

// extractParamStem strips common ID suffixes from a path parameter name.
func extractParamStem(name string) string {
	for _, suffix := range []string{"_id", "_ID", "Id", "ID", "_number", "Number", "_no", "No"} {
		if strings.HasSuffix(name, suffix) && len(name) > len(suffix) {
			stem := name[:len(name)-len(suffix)]
			return strings.TrimRight(stem, "_")
		}
	}
	return ""
}

// matchParamNameToDomain tries to match a normalized name to a domain
// using pluralization and kebab/underscore normalization.
func matchParamNameToDomain(name string, domainSet map[string]string) (string, bool) {
	if orig, ok := domainSet[name]; ok {
		return orig, true
	}
	if orig, ok := domainSet[name+"s"]; ok {
		return orig, true
	}
	if orig, ok := domainSet[name+"es"]; ok {
		return orig, true
	}
	if strings.HasSuffix(name, "y") {
		if orig, ok := domainSet[name[:len(name)-1]+"ies"]; ok {
			return orig, true
		}
	}
	kebab := paramCamelToKebab(name)
	if kebab != name {
		if orig, ok := domainSet[kebab]; ok {
			return orig, true
		}
		if orig, ok := domainSet[kebab+"s"]; ok {
			return orig, true
		}
		if orig, ok := domainSet[kebab+"es"]; ok {
			return orig, true
		}
	}
	normName := strings.ReplaceAll(strings.ReplaceAll(name, "-", ""), "_", "")
	for domLower, orig := range domainSet {
		normDom := strings.ReplaceAll(strings.ReplaceAll(domLower, "-", ""), "_", "")
		if normDom == normName || normDom == normName+"s" || normDom == normName+"es" {
			return orig, true
		}
	}
	return "", false
}

func paramCamelToKebab(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result = append(result, '-')
			}
			result = append(result, c|0x20)
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

func (r *PlanRunner) computeSummary(result *RunResult) {
	for _, dr := range result.Domains {
		result.Summary.TotalDomains++
		if dr.Success {
			result.Summary.PassedDomains++
		} else {
			result.Summary.FailedDomains++
		}
		for _, op := range dr.Operations {
			result.Summary.TotalOps++
			if op.Skipped {
				result.Summary.SkippedOps++
			} else if op.Success {
				result.Summary.PassedOps++
			} else {
				result.Summary.FailedOps++
			}
		}
	}
}

// parsedReq is an internal representation of a parsed .request file.
type parsedReq struct {
	method  string
	path    string
	headers map[string]string
	body    string
}

// parseRequestFile reads and parses a .request file from disk.
func parseRequestFile(path string) (*parsedReq, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading request file: %w", err)
	}
	return parseRequestString(string(data))
}

// parseRequestString parses an already-resolved .request content string.
func parseRequestString(content string) (*parsedReq, error) {
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty request file")
	}

	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid request line: %s", lines[0])
	}

	req := &parsedReq{
		method:  parts[0],
		path:    parts[1],
		headers: make(map[string]string),
	}

	if len(lines) < 2 {
		return req, nil
	}

	remaining := lines[1]
	headerBodySplit := strings.SplitN(remaining, "\n\n", 2)

	for _, line := range strings.Split(headerBodySplit[0], "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		colonIdx := strings.Index(line, ": ")
		if colonIdx == -1 {
			continue
		}
		req.headers[line[:colonIdx]] = line[colonIdx+2:]
	}

	if len(headerBodySplit) > 1 {
		req.body = strings.TrimSpace(headerBodySplit[1])
	}

	return req, nil
}

// domainFromFilename extracts the domain name from a request filename.
func domainFromFilename(filename string) string {
	name := strings.TrimSuffix(filename, ".request")
	for _, method := range []string{"_DELETE", "_PATCH", "_POST", "_PUT", "_GET"} {
		if idx := strings.Index(name, method); idx != -1 {
			return name[:idx]
		}
	}
	return name
}

// injectVersionFields merges version fields into a JSON body.
func injectVersionFields(body string, versionFields map[string]interface{}) string {
	if len(versionFields) == 0 {
		return body
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return body
	}
	for k, v := range versionFields {
		data[k] = v
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return body
	}
	return string(out)
}

// injectFieldInBody adds or updates a field in a JSON body.
func injectFieldInBody(body, field string, value interface{}) string {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return body
	}
	data[field] = value
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return body
	}
	return string(out)
}

// inferExpectedStatus returns a reasonable expected status for an HTTP method.
func inferExpectedStatus(method string) int {
	switch method {
	case "POST":
		return 201
	case "DELETE":
		return 204
	default:
		return 200
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "... (truncated)"
}

// isAcceptableStatus checks if a status code is acceptable for a given method.
func isAcceptableStatus(method string, status int) bool {
	switch method {
	case "POST":
		return status == 200 || status == 201
	case "DELETE":
		return status == 200 || status == 202 || status == 204
	case "PUT", "PATCH":
		return status == 200 || status == 204
	default:
		return status == 200
	}
}
