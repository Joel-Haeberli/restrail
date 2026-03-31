package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"restrail/internal/openapi"
	"testing"
)

func TestNewAuthenticator_NoCreds(t *testing.T) {
	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"keycloak": {
					Type:             "openIdConnect",
					OpenIDConnectURL: "https://example.com/.well-known/openid-configuration",
				},
			},
		},
	}

	// All four credential fields empty → none
	auth := NewAuthenticator(spec, "", "", "", "")
	if auth.Name() != "none" {
		t.Errorf("expected none authenticator, got %s", auth.Name())
	}
}

func TestNewAuthenticator_BasicAuth(t *testing.T) {
	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"basic": {
					Type:   "http",
					Scheme: "basic",
				},
			},
		},
	}

	auth := NewAuthenticator(spec, "admin", "password", "", "")
	if auth.Name() != "basic" {
		t.Errorf("expected basic authenticator, got %s", auth.Name())
	}
	ba, ok := auth.(*basicAuth)
	if !ok {
		t.Fatal("expected *basicAuth type")
	}
	if ba.username != "admin" || ba.password != "password" {
		t.Errorf("expected admin/password, got %s/%s", ba.username, ba.password)
	}
}

func TestNewAuthenticator_BearerAuth(t *testing.T) {
	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"bearer": {
					Type:   "http",
					Scheme: "bearer",
				},
			},
		},
	}

	auth := NewAuthenticator(spec, "", "my-token", "", "")
	if auth.Name() != "bearer" {
		t.Errorf("expected bearer authenticator, got %s", auth.Name())
	}
	ba, ok := auth.(*bearerAuth)
	if !ok {
		t.Fatal("expected *bearerAuth type")
	}
	if ba.token != "my-token" {
		t.Errorf("expected my-token, got %s", ba.token)
	}
}

func TestNewAuthenticator_OAuth2_ClientCredentials_ViaSubject(t *testing.T) {
	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"oauth": {
					Type: "oauth2",
					Flows: &openapi.OAuthFlows{
						ClientCredentials: &openapi.OAuthFlow{
							TokenURL: "https://example.com/token",
						},
					},
				},
			},
		},
	}

	// No clientID → client_credentials using subject/secret as client credentials
	auth := NewAuthenticator(spec, "my-client-id", "my-client-secret", "", "")
	if auth.Name() != "oauth2" {
		t.Errorf("expected oauth2 authenticator, got %s", auth.Name())
	}
	oa, ok := auth.(*oauth2Auth)
	if !ok {
		t.Fatal("expected *oauth2Auth type")
	}
	if oa.grantType != "client_credentials" {
		t.Errorf("expected client_credentials grant, got %s", oa.grantType)
	}
	if oa.clientID != "my-client-id" {
		t.Errorf("expected clientID=my-client-id, got %s", oa.clientID)
	}
	if oa.clientSecret != "my-client-secret" {
		t.Errorf("expected clientSecret=my-client-secret, got %s", oa.clientSecret)
	}
}

func TestNewAuthenticator_OAuth2_ClientCredentials_ViaClientID(t *testing.T) {
	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"oauth": {
					Type: "oauth2",
					Flows: &openapi.OAuthFlows{
						ClientCredentials: &openapi.OAuthFlow{
							TokenURL: "https://example.com/token",
						},
					},
				},
			},
		},
	}

	// clientID set but no subject → client_credentials using clientID/clientSecret directly
	auth := NewAuthenticator(spec, "", "", "my-client-id", "my-client-secret")
	if auth.Name() != "oauth2" {
		t.Errorf("expected oauth2 authenticator, got %s", auth.Name())
	}
	oa, ok := auth.(*oauth2Auth)
	if !ok {
		t.Fatal("expected *oauth2Auth type")
	}
	if oa.grantType != "client_credentials" {
		t.Errorf("expected client_credentials grant, got %s", oa.grantType)
	}
	if oa.clientID != "my-client-id" {
		t.Errorf("expected clientID=my-client-id, got %s", oa.clientID)
	}
	if oa.clientSecret != "my-client-secret" {
		t.Errorf("expected clientSecret=my-client-secret, got %s", oa.clientSecret)
	}
}

func TestNewAuthenticator_OAuth2_PasswordGrant(t *testing.T) {
	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"oauth": {
					Type: "oauth2",
					Flows: &openapi.OAuthFlows{
						ClientCredentials: &openapi.OAuthFlow{
							TokenURL: "https://example.com/token",
						},
					},
				},
			},
		},
	}

	// Both clientID and subject set → password grant
	auth := NewAuthenticator(spec, "admin", "admin-pass", "my-client-id", "my-client-secret")
	if auth.Name() != "oauth2" {
		t.Errorf("expected oauth2 authenticator, got %s", auth.Name())
	}
	oa, ok := auth.(*oauth2Auth)
	if !ok {
		t.Fatal("expected *oauth2Auth type")
	}
	if oa.grantType != "password" {
		t.Errorf("expected password grant, got %s", oa.grantType)
	}
	if oa.username != "admin" {
		t.Errorf("expected username=admin, got %s", oa.username)
	}
	if oa.password != "admin-pass" {
		t.Errorf("expected password=admin-pass, got %s", oa.password)
	}
	if oa.clientID != "my-client-id" {
		t.Errorf("expected clientID=my-client-id, got %s", oa.clientID)
	}
	if oa.clientSecret != "my-client-secret" {
		t.Errorf("expected clientSecret=my-client-secret, got %s", oa.clientSecret)
	}
}

func TestNewAuthenticator_OpenIDConnect_Discovery(t *testing.T) {
	// Serve a real OpenID Connect discovery document
	discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"token_endpoint": "https://keycloak.example.com/realms/test/protocol/openid-connect/token",
			"issuer":         "https://keycloak.example.com/realms/test",
		})
	}))
	defer discoveryServer.Close()

	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"keycloak": {
					Type:             "openIdConnect",
					OpenIDConnectURL: discoveryServer.URL,
				},
			},
		},
	}

	// Should discover the token_endpoint from the discovery doc
	auth := NewAuthenticator(spec, "", "", "my-client-id", "my-client-secret")
	if auth.Name() != "oauth2" {
		t.Errorf("expected oauth2 authenticator, got %s", auth.Name())
	}
	oa, ok := auth.(*oauth2Auth)
	if !ok {
		t.Fatal("expected *oauth2Auth type")
	}
	if oa.tokenURL != "https://keycloak.example.com/realms/test/protocol/openid-connect/token" {
		t.Errorf("expected discovered token URL, got %s", oa.tokenURL)
	}
	if oa.grantType != "client_credentials" {
		t.Errorf("expected client_credentials grant, got %s", oa.grantType)
	}
}

func TestNewAuthenticator_OpenIDConnect_PasswordGrant(t *testing.T) {
	discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"token_endpoint": "https://keycloak.example.com/realms/test/protocol/openid-connect/token",
		})
	}))
	defer discoveryServer.Close()

	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"keycloak": {
					Type:             "openIdConnect",
					OpenIDConnectURL: discoveryServer.URL,
				},
			},
		},
	}

	// Both subject and clientID → password grant
	auth := NewAuthenticator(spec, "admin", "admin-pass", "my-client-id", "my-client-secret")
	if auth.Name() != "oauth2" {
		t.Errorf("expected oauth2 authenticator, got %s", auth.Name())
	}
	oa, ok := auth.(*oauth2Auth)
	if !ok {
		t.Fatal("expected *oauth2Auth type")
	}
	if oa.grantType != "password" {
		t.Errorf("expected password grant, got %s", oa.grantType)
	}
	if oa.username != "admin" {
		t.Errorf("expected username=admin, got %s", oa.username)
	}
	if oa.clientID != "my-client-id" {
		t.Errorf("expected clientID=my-client-id, got %s", oa.clientID)
	}
	if oa.tokenURL != "https://keycloak.example.com/realms/test/protocol/openid-connect/token" {
		t.Errorf("expected discovered token URL, got %s", oa.tokenURL)
	}
}

func TestNewAuthenticator_OpenIDConnect_DiscoveryFails_FallsBackToURL(t *testing.T) {
	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{
				"keycloak": {
					Type:             "openIdConnect",
					OpenIDConnectURL: "http://localhost:1/nonexistent",
				},
			},
		},
	}

	// Discovery fails → uses the URL directly as token endpoint
	auth := NewAuthenticator(spec, "", "", "my-client-id", "my-client-secret")
	if auth.Name() != "oauth2" {
		t.Errorf("expected oauth2 authenticator, got %s", auth.Name())
	}
	oa, ok := auth.(*oauth2Auth)
	if !ok {
		t.Fatal("expected *oauth2Auth type")
	}
	if oa.tokenURL != "http://localhost:1/nonexistent" {
		t.Errorf("expected fallback token URL, got %s", oa.tokenURL)
	}
}

func TestNewAuthenticator_FallbackToBasic(t *testing.T) {
	// No security schemes but credentials provided → fallback to basic
	spec := &openapi.Spec{
		Components: openapi.Components{
			SecuritySchemes: map[string]openapi.SecurityScheme{},
		},
	}

	auth := NewAuthenticator(spec, "user", "pass", "", "")
	if auth.Name() != "basic" {
		t.Errorf("expected basic fallback, got %s", auth.Name())
	}
}

func TestDiscoverTokenURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"token_endpoint": "https://example.com/token",
			"issuer":         "https://example.com",
		})
	}))
	defer server.Close()

	tokenURL, err := discoverTokenURL(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokenURL != "https://example.com/token" {
		t.Errorf("expected https://example.com/token, got %s", tokenURL)
	}
}
