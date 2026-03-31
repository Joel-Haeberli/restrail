package profile

import (
	"restrail/internal/openapi"
	"strings"
)

// DomainMatch represents a matched domain with its spec paths mapped to operations.
type DomainMatch struct {
	Domain     string
	PathPrefix string
	Mappings   map[string]OperationMapping // operation name -> mapping
}

// OperationMapping maps a profile operation to a concrete spec path and operation.
type OperationMapping struct {
	Operation *Operation
	SpecPath  string
	SpecOp    *openapi.Operation
	IDParam   string // name of the path parameter used as ID (e.g., "userId")
}

// DiscoverResult holds the result of profile discovery.
type DiscoverResult struct {
	ProfileName    string
	MatchedDomains int
	TotalDomains   int
	Domains        []DomainMatch
}

// DiscoverBest tries all profiles and returns the best match.
func DiscoverBest(spec *openapi.Spec, profiles []*Profile) (*DiscoverResult, bool) {
	var best *DiscoverResult
	var bestOps int
	for _, p := range profiles {
		domains := MatchProfile(spec, p)
		if len(domains) == 0 {
			continue
		}
		// Count total matched operations across all domains
		totalOps := 0
		for _, d := range domains {
			totalOps += len(d.Mappings)
		}
		result := &DiscoverResult{
			ProfileName:    p.Name,
			MatchedDomains: len(domains),
			TotalDomains:   len(domains),
			Domains:        domains,
		}
		// Prefer more matched domains; break ties by total matched operations
		// (more specific profiles like "strict" win over "appendonly")
		if best == nil || result.MatchedDomains > best.MatchedDomains ||
			(result.MatchedDomains == best.MatchedDomains && totalOps > bestOps) {
			best = result
			bestOps = totalOps
		}
	}
	return best, best != nil
}

// MatchProfile matches a profile against the spec and returns matched domains.
func MatchProfile(spec *openapi.Spec, p *Profile) []DomainMatch {
	prefix := commonPathPrefix(spec)
	groups := groupPathsByDomain(spec, prefix)

	var matches []DomainMatch
	for domain, paths := range groups {
		match, ok := matchDomain(spec, p, domain, prefix, paths)
		if ok {
			matches = append(matches, match)
		}
	}
	return matches
}

func commonPathPrefix(spec *openapi.Spec) string {
	var paths []string
	for p := range spec.Paths {
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		// Single path: prefix is everything before the last segment
		idx := strings.LastIndex(paths[0], "/")
		if idx <= 0 {
			return ""
		}
		return paths[0][:idx]
	}

	prefix := paths[0]
	for _, p := range paths[1:] {
		for !strings.HasPrefix(p, prefix) {
			idx := strings.LastIndex(prefix, "/")
			if idx <= 0 {
				return ""
			}
			prefix = prefix[:idx]
		}
	}
	// Ensure prefix ends at a segment boundary
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		idx := strings.LastIndex(prefix, "/")
		if idx <= 0 {
			return ""
		}
		prefix = prefix[:idx]
	}
	return prefix
}

type pathGroup struct {
	specPath string
	relative string
}

func groupPathsByDomain(spec *openapi.Spec, prefix string) map[string][]pathGroup {
	groups := make(map[string][]pathGroup)
	for specPath := range spec.Paths {
		relative := strings.TrimPrefix(specPath, prefix)
		relative = strings.TrimPrefix(relative, "/")
		segments := strings.Split(relative, "/")
		if len(segments) == 0 || segments[0] == "" {
			continue
		}
		domain := segments[0]
		groups[domain] = append(groups[domain], pathGroup{
			specPath: specPath,
			relative: "/" + relative,
		})
	}
	return groups
}

func matchDomain(spec *openapi.Spec, p *Profile, domain, prefix string, paths []pathGroup) (DomainMatch, bool) {
	match := DomainMatch{
		Domain:     domain,
		PathPrefix: prefix,
		Mappings:   make(map[string]OperationMapping),
	}

	requiredMatched := 0
	requiredTotal := 0

	for i := range p.Operations {
		op := &p.Operations[i]
		if !op.Optional {
			requiredTotal++
		}

		mapped := false
		for _, pg := range paths {
			if !patternMatches(op.Pattern, pg.relative, domain) {
				continue
			}
			specItem := spec.Paths[pg.specPath]
			if specItem == nil {
				continue
			}
			specOp := specItem.MethodOperation(op.Method)
			if specOp == nil {
				continue
			}
			idParam := extractIDParam(pg.relative, op.Pattern)
			match.Mappings[op.Name] = OperationMapping{
				Operation: op,
				SpecPath:  pg.specPath,
				SpecOp:    specOp,
				IDParam:   idParam,
			}
			mapped = true
			if !op.Optional {
				requiredMatched++
			}
			break
		}
		if !mapped && !op.Optional {
			return match, false
		}
	}

	return match, requiredMatched == requiredTotal
}

// patternMatches checks if a relative spec path matches a profile pattern.
// Pattern: /{domain} matches /users, /{domain}/{id} matches /users/{userId}
func patternMatches(pattern, relativePath, domain string) bool {
	patternSegments := splitSegments(pattern)
	pathSegments := splitSegments(relativePath)

	if len(patternSegments) != len(pathSegments) {
		return false
	}

	for i, ps := range patternSegments {
		rs := pathSegments[i]
		switch {
		case ps == "{domain}":
			if rs != domain {
				return false
			}
		case ps == "{id}":
			// Matches any path parameter (e.g., {userId})
			if !isPathParam(rs) {
				return false
			}
		default:
			if ps != rs {
				return false
			}
		}
	}
	return true
}

func splitSegments(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func isPathParam(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

func extractIDParam(relativePath, pattern string) string {
	patternSegments := splitSegments(pattern)
	pathSegments := splitSegments(relativePath)
	for i, ps := range patternSegments {
		if ps == "{id}" && i < len(pathSegments) {
			param := pathSegments[i]
			return strings.Trim(param, "{}")
		}
	}
	return ""
}
