package openapi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func ParseFile(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec file: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (*Spec, error) {
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing spec JSON: %w", err)
	}
	if spec.Paths == nil {
		spec.Paths = make(map[string]*PathItem)
	}
	if spec.Components.Schemas == nil {
		spec.Components.Schemas = make(map[string]*Schema)
	}
	if spec.Components.SecuritySchemes == nil {
		spec.Components.SecuritySchemes = make(map[string]SecurityScheme)
	}
	resolveRefs(&spec)
	return &spec, nil
}

func resolveRefs(spec *Spec) {
	visited := make(map[string]bool)
	for path, item := range spec.Paths {
		_ = path
		resolvePathItemRefs(item, spec, visited)
	}
}

func resolvePathItemRefs(item *PathItem, spec *Spec, visited map[string]bool) {
	ops := []*Operation{item.Get, item.Post, item.Put, item.Delete, item.Patch, item.Options, item.Head}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if op.RequestBody != nil {
			for _, mt := range op.RequestBody.Content {
				if mt.Schema != nil {
					resolveSchemaRefs(mt.Schema, spec, visited)
				}
			}
		}
		for _, resp := range op.Responses {
			if resp == nil {
				continue
			}
			for _, mt := range resp.Content {
				if mt.Schema != nil {
					resolveSchemaRefs(mt.Schema, spec, visited)
				}
			}
		}
		for i := range op.Parameters {
			if op.Parameters[i].Schema != nil {
				resolveSchemaRefs(op.Parameters[i].Schema, spec, visited)
			}
		}
	}
}

func resolveSchemaRefs(schema *Schema, spec *Spec, visited map[string]bool) {
	if schema == nil {
		return
	}
	if schema.Ref != "" {
		ref := schema.Ref
		if visited[ref] {
			// Circular reference — clear the ref but don't recurse
			schema.Ref = ""
			return
		}
		visited[ref] = true
		resolved := lookupRef(ref, spec)
		if resolved != nil {
			resolveSchemaRefs(resolved, spec, visited)
			*schema = *resolved
			schema.Ref = ""
		}
		delete(visited, ref)
		return
	}
	for _, prop := range schema.Properties {
		resolveSchemaRefs(prop, spec, visited)
	}
	if schema.Items != nil {
		resolveSchemaRefs(schema.Items, spec, visited)
	}
	for _, s := range schema.AllOf {
		resolveSchemaRefs(s, spec, visited)
	}
	for _, s := range schema.OneOf {
		resolveSchemaRefs(s, spec, visited)
	}
	for _, s := range schema.AnyOf {
		resolveSchemaRefs(s, spec, visited)
	}
}

func lookupRef(ref string, spec *Spec) *Schema {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	if len(parts) == 3 && parts[0] == "components" && parts[1] == "schemas" {
		name := parts[2]
		if s, ok := spec.Components.Schemas[name]; ok {
			return deepCopySchema(s)
		}
	}
	return nil
}

func deepCopySchema(s *Schema) *Schema {
	if s == nil {
		return nil
	}
	cp := *s
	if s.Properties != nil {
		cp.Properties = make(map[string]*Schema, len(s.Properties))
		for k, v := range s.Properties {
			cp.Properties[k] = deepCopySchema(v)
		}
	}
	cp.Items = deepCopySchema(s.Items)
	if s.Required != nil {
		cp.Required = make([]string, len(s.Required))
		copy(cp.Required, s.Required)
	}
	if s.Enum != nil {
		cp.Enum = make([]json.RawMessage, len(s.Enum))
		copy(cp.Enum, s.Enum)
	}
	if s.AllOf != nil {
		cp.AllOf = make([]*Schema, len(s.AllOf))
		for i, sub := range s.AllOf {
			cp.AllOf[i] = deepCopySchema(sub)
		}
	}
	if s.OneOf != nil {
		cp.OneOf = make([]*Schema, len(s.OneOf))
		for i, sub := range s.OneOf {
			cp.OneOf[i] = deepCopySchema(sub)
		}
	}
	if s.AnyOf != nil {
		cp.AnyOf = make([]*Schema, len(s.AnyOf))
		for i, sub := range s.AnyOf {
			cp.AnyOf[i] = deepCopySchema(sub)
		}
	}
	return &cp
}

// SecurityInfo describes the resolved security for an operation.
type SecurityInfo struct {
	SchemeName string `json:"scheme_name"`
	Type       string `json:"type"`
	Scheme     string `json:"scheme,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
}

// ResolveOperationSecurity returns the security schemes that apply to an operation.
// Operation-level security overrides global security per the OpenAPI spec.
func ResolveOperationSecurity(spec *Spec, op *Operation) []SecurityInfo {
	requirements := op.Security
	if requirements == nil {
		requirements = spec.Security
	}
	if len(requirements) == 0 {
		return nil
	}

	var infos []SecurityInfo
	for _, req := range requirements {
		for schemeName, scopes := range req {
			scheme, ok := spec.Components.SecuritySchemes[schemeName]
			if !ok {
				infos = append(infos, SecurityInfo{
					SchemeName: schemeName,
					Type:       "unknown",
					Scopes:     scopes,
				})
				continue
			}
			info := SecurityInfo{
				SchemeName: schemeName,
				Type:       scheme.Type,
				Scopes:     scopes,
			}
			if scheme.Type == "http" {
				info.Scheme = scheme.Scheme
			}
			infos = append(infos, info)
		}
	}
	return infos
}

// GetRequestBodySchema returns the JSON schema for a request body, if any.
func GetRequestBodySchema(op *Operation) *Schema {
	if op == nil || op.RequestBody == nil {
		return nil
	}
	if mt, ok := op.RequestBody.Content["application/json"]; ok {
		return mt.Schema
	}
	// Fall back to first content type
	for _, mt := range op.RequestBody.Content {
		return mt.Schema
	}
	return nil
}

// GetResponseSchema returns the JSON schema for a given response status code.
func GetResponseSchema(op *Operation, status string) *Schema {
	if op == nil || op.Responses == nil {
		return nil
	}
	resp, ok := op.Responses[status]
	if !ok {
		return nil
	}
	if resp == nil || resp.Content == nil {
		return nil
	}
	if mt, ok := resp.Content["application/json"]; ok {
		return mt.Schema
	}
	for _, mt := range resp.Content {
		return mt.Schema
	}
	return nil
}
