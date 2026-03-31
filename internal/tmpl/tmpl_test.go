package tmpl

import (
	"strings"
	"testing"
)

func TestFKStringID(t *testing.T) {
	registry := map[string]interface{}{"customers": "abc-123"}
	funcMap := NewFuncMap(registry, func(p string) string { return p })

	content := `{"customerId": {{ fk "customers" }}}`
	result, err := Execute("test", content, funcMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `{"customerId": "abc-123"}` {
		t.Errorf("got %q, want %q", result, `{"customerId": "abc-123"}`)
	}
}

func TestFKNumericID(t *testing.T) {
	registry := map[string]interface{}{"orders": float64(42)}
	funcMap := NewFuncMap(registry, func(p string) string { return p })

	content := `{"orderId": {{ fk "orders" }}}`
	result, err := Execute("test", content, funcMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `{"orderId": 42}` {
		t.Errorf("got %q, want %q", result, `{"orderId": 42}`)
	}
}

func TestFKMissingDomain(t *testing.T) {
	registry := map[string]interface{}{}
	funcMap := NewFuncMap(registry, func(p string) string { return p })

	content := `{"customerId": {{ fk "customers" }}}`
	_, err := Execute("test", content, funcMap)
	if err == nil {
		t.Fatal("expected error for missing domain")
	}
}

func TestParamResolution(t *testing.T) {
	resolver := func(rawPath string) string {
		return strings.Replace(rawPath, "{orderId}", "42", 1)
	}
	funcMap := NewFuncMap(nil, resolver)

	content := `GET {{ param "/api/v1/orders/{orderId}" }} HTTP/1.1`
	result, err := Execute("test", content, funcMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `GET /api/v1/orders/42 HTTP/1.1` {
		t.Errorf("got %q", result)
	}
}

func TestFKOptionalMissingDomain(t *testing.T) {
	registry := map[string]interface{}{}
	funcMap := NewFuncMap(registry, func(p string) string { return p })

	// fk_optional emits the OmitMarker JSON string so RemoveOmitFields can strip the field.
	content := `{"location": {"id": {{ fk_optional "locations" }}}}`
	result, err := Execute("test", content, funcMap)
	if err != nil {
		t.Fatalf("expected no error for fk_optional with missing domain, got: %v", err)
	}
	want := `{"location": {"id": "` + OmitMarker + `"}}`
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}

	// After RemoveOmitFields the field is omitted entirely.
	cleaned, err := RemoveOmitFields(result)
	if err != nil {
		t.Fatalf("RemoveOmitFields error: %v", err)
	}
	if cleaned != `{}` {
		t.Errorf("after RemoveOmitFields got %q, want {}", cleaned)
	}
}

func TestFKOptionalPresentDomain(t *testing.T) {
	registry := map[string]interface{}{"locations": "loc-uuid-123"}
	funcMap := NewFuncMap(registry, func(p string) string { return p })

	content := `{"location": {"id": {{ fk_optional "locations" }}}}`
	result, err := Execute("test", content, funcMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `{"location": {"id": "loc-uuid-123"}}` {
		t.Errorf("got %q", result)
	}
}

func TestFKOptionalSentinelConversion(t *testing.T) {
	input := `{"location": {"id": "__FK_OPT:locations__"}, "building": {"id": "__FK:buildings__"}}`
	got := ConvertSentinels(input)
	if !strings.Contains(got, `{{ fk_optional "locations" }}`) {
		t.Errorf("expected fk_optional template, got: %s", got)
	}
	if !strings.Contains(got, `{{ fk "buildings" }}`) {
		t.Errorf("expected fk template, got: %s", got)
	}
	if strings.Contains(got, "__FK_OPT:") || strings.Contains(got, "__FK:") {
		t.Errorf("sentinels not fully replaced: %s", got)
	}
}

func TestConvertPathToTemplate(t *testing.T) {
	// Path with params
	got := ConvertPathToTemplate("/api/v1/orders/{orderId}")
	want := `{{ param "/api/v1/orders/{orderId}" }}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Path without params — left unchanged
	got = ConvertPathToTemplate("/api/v1/orders")
	if got != "/api/v1/orders" {
		t.Errorf("got %q, want unchanged path", got)
	}
}

func TestFKSentinelConversion(t *testing.T) {
	input := `{"customerId": "__FK:customers__", "productId": "__FK:products__"}`
	got := ConvertSentinels(input)
	if !strings.Contains(got, `{{ fk "customers" }}`) {
		t.Errorf("expected fk customers template, got: %s", got)
	}
	if !strings.Contains(got, `{{ fk "products" }}`) {
		t.Errorf("expected fk products template, got: %s", got)
	}
	if strings.Contains(got, "__FK:") {
		t.Errorf("sentinel not fully replaced: %s", got)
	}
}

func TestFKSentinelWithHyphenatedDomain(t *testing.T) {
	input := `{"postalCode": "__FK:postal-codes__"}`
	got := ConvertSentinels(input)
	want := `{"postalCode": {{ fk "postal-codes" }}}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFullRequestTemplate(t *testing.T) {
	registry := map[string]interface{}{
		"customers":    "cust-1",
		"postal-codes": float64(8001),
	}
	resolver := func(rawPath string) string {
		return strings.Replace(rawPath, "{customerId}", "cust-1", 1)
	}
	funcMap := NewFuncMap(registry, resolver)

	content := `PUT {{ param "/api/v1/customers/{customerId}" }} HTTP/1.1
Accept: application/json
Content-Type: application/json
Authorization: __AUTH__

{
  "name": "test",
  "postalCode": {{ fk "postal-codes" }},
  "referrerId": {{ fk "customers" }}
}`

	result, err := Execute("test", content, funcMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "PUT /api/v1/customers/cust-1 HTTP/1.1") {
		t.Errorf("path not resolved: %s", result)
	}
	if !strings.Contains(result, `"postalCode": 8001`) {
		t.Errorf("numeric FK not resolved: %s", result)
	}
	if !strings.Contains(result, `"referrerId": "cust-1"`) {
		t.Errorf("string FK not resolved: %s", result)
	}
	if !strings.Contains(result, "Authorization: __AUTH__") {
		t.Errorf("auth marker missing: %s", result)
	}
}
