package tmpl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// OmitMarker is the sentinel value returned by fk_optional when the target
// domain is not in the ID registry. RemoveOmitFields scans a JSON body and
// removes any field whose value contains this marker.
const OmitMarker = "__RESTRAIL_OMIT__"

// NewFuncMap builds a template.FuncMap with the fk and param functions.
//
// "fk" resolves a domain name to its created resource ID, returning the
// JSON-encoded value (preserving type: "abc" for strings, 123 for numbers).
//
// "param" resolves all {paramName} placeholders in a path string using
// the provided resolver function.
func NewFuncMap(idRegistry map[string]interface{}, paramResolver func(string) string) template.FuncMap {
	return template.FuncMap{
		"fk": func(domain string) (string, error) {
			rawID, ok := idRegistry[domain]
			if !ok {
				return "", fmt.Errorf("fk: domain %q not found in ID registry", domain)
			}
			b, err := json.Marshal(rawID)
			if err != nil {
				return "", fmt.Errorf("fk: marshalling ID for domain %q: %w", domain, err)
			}
			return string(b), nil
		},
		"fk_optional": func(domain string) (string, error) {
			rawID, ok := idRegistry[domain]
			if !ok {
				return `"` + OmitMarker + `"`, nil
			}
			b, err := json.Marshal(rawID)
			if err != nil {
				return "", fmt.Errorf("fk_optional: marshalling ID for domain %q: %w", domain, err)
			}
			return string(b), nil
		},
		"param": func(rawPath string) string {
			return paramResolver(rawPath)
		},
	}
}

// Execute parses and executes a .request template with the given FuncMap.
// Returns the fully resolved content string.
func Execute(name, content string, funcMap template.FuncMap) (string, error) {
	t, err := template.New(name).Funcs(funcMap).Parse(content)
	if err != nil {
		return "", fmt.Errorf("parsing template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, nil); err != nil {
		return "", fmt.Errorf("executing template %s: %w", name, err)
	}

	return buf.String(), nil
}

// ConvertPathToTemplate converts a raw spec path like /api/v1/orders/{orderId}
// into a Go template expression: {{ param "/api/v1/orders/{orderId}" }}
// Only converts paths that contain {param} placeholders.
func ConvertPathToTemplate(path string) string {
	if !strings.Contains(path, "{") {
		return path
	}
	return fmt.Sprintf(`{{ param "%s" }}`, path)
}

// FKSentinel returns the sentinel string placed into the body map before
// JSON marshaling. After marshaling, ConvertSentinels replaces these with
// Go template expressions that call the strict "fk" function.
func FKSentinel(domain string) string {
	return "__FK:" + domain + "__"
}

// FKOptionalSentinel returns the sentinel string for optional FK references.
// After marshaling, ConvertSentinels replaces these with template expressions
// that call "fk_optional", which returns null instead of erroring when
// the target domain's ID is unavailable.
func FKOptionalSentinel(domain string) string {
	return "__FK_OPT:" + domain + "__"
}

// ConvertSentinels replaces FK sentinel strings in JSON output with
// Go template expressions. Handles both required (__FK:) and optional
// (__FK_OPT:) sentinels.
func ConvertSentinels(jsonStr string) string {
	result := jsonStr
	// Process required FK sentinels: "__FK:domain__" -> {{ fk "domain" }}
	result = convertSentinelType(result, "__FK:", "fk")
	// Process optional FK sentinels: "__FK_OPT:domain__" -> {{ fk_optional "domain" }}
	result = convertSentinelType(result, "__FK_OPT:", "fk_optional")
	return result
}

// RemoveOmitFields parses a JSON object string and removes any field whose
// value contains OmitMarker (directly or nested inside an object or array).
// This is used after fk_optional template execution to drop optional FK fields
// that have no registry value rather than sending them as null.
// Returns the input unchanged if it is not a valid JSON object.
func RemoveOmitFields(jsonStr string) (string, error) {
	if jsonStr == "" {
		return jsonStr, nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return jsonStr, nil
	}
	removeOmitMarkers(data)
	b, err := json.Marshal(data)
	if err != nil {
		return jsonStr, err
	}
	return string(b), nil
}

func removeOmitMarkers(data map[string]interface{}) {
	for key, val := range data {
		if containsOmitMarker(val) {
			delete(data, key)
		} else if nested, ok := val.(map[string]interface{}); ok {
			removeOmitMarkers(nested)
		}
	}
}

func containsOmitMarker(val interface{}) bool {
	switch v := val.(type) {
	case string:
		return v == OmitMarker
	case map[string]interface{}:
		for _, inner := range v {
			if containsOmitMarker(inner) {
				return true
			}
		}
	case []interface{}:
		for _, item := range v {
			if containsOmitMarker(item) {
				return true
			}
		}
	}
	return false
}

func convertSentinelType(jsonStr, prefix, funcName string) string {
	result := jsonStr
	searchPrefix := `"` + prefix
	prefixLen := len(prefix)
	for {
		start := strings.Index(result, searchPrefix)
		if start == -1 {
			break
		}
		// Find the closing __" after the prefix
		afterPrefix := start + 1 + prefixLen // skip opening quote + prefix
		end := strings.Index(result[afterPrefix:], `__"`)
		if end == -1 {
			break
		}
		domain := result[afterPrefix : afterPrefix+end]
		sentinel := fmt.Sprintf(`"%s%s__"`, prefix, domain)
		replacement := fmt.Sprintf(`{{ %s "%s" }}`, funcName, domain)
		result = strings.Replace(result, sentinel, replacement, 1)
	}
	return result
}
