package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"restrail/internal/openapi"
	"strings"
	"sync"
	"time"
)

// Authenticator adds authentication to HTTP requests.
type Authenticator interface {
	Authenticate(req *http.Request) error
	Name() string
}

// NewAuthenticator auto-detects the auth type from the spec and credentials.
// When clientID is set, OAuth2 uses the password grant (subject=username, secret=password).
// When clientID is empty, OAuth2 uses client_credentials (subject=client_id, secret=client_secret).
func NewAuthenticator(spec *openapi.Spec, subject, secret, clientID, clientSecret string) Authenticator {
	if subject == "" && secret == "" && clientID == "" && clientSecret == "" {
		return &noneAuth{}
	}

	for _, scheme := range spec.Components.SecuritySchemes {
		switch scheme.Type {
		case "oauth2":
			tokenURL := extractTokenURL(scheme)
			if tokenURL != "" {
				return newOAuth2Auth(tokenURL, subject, secret, clientID, clientSecret)
			}
		case "openIdConnect":
			if scheme.OpenIDConnectURL != "" {
				tokenURL, err := discoverTokenURL(scheme.OpenIDConnectURL)
				if err != nil || tokenURL == "" {
					// Fall back to using the URL directly as token endpoint
					tokenURL = scheme.OpenIDConnectURL
				}
				return newOAuth2Auth(tokenURL, subject, secret, clientID, clientSecret)
			}
		case "http":
			if strings.EqualFold(scheme.Scheme, "basic") {
				return &basicAuth{username: subject, password: secret}
			}
			if strings.EqualFold(scheme.Scheme, "bearer") {
				return &bearerAuth{token: secret}
			}
		}
	}

	// Default to basic auth when credentials are provided but no scheme matched
	return &basicAuth{username: subject, password: secret}
}

func newOAuth2Auth(tokenURL, subject, secret, clientID, clientSecret string) *oauth2Auth {
	if clientID != "" && subject != "" {
		// Password grant: clientID identifies the OAuth client, subject/secret are user credentials
		return &oauth2Auth{
			tokenURL:     tokenURL,
			grantType:    "password",
			clientID:     clientID,
			clientSecret: clientSecret,
			username:     subject,
			password:     secret,
		}
	}
	// Client credentials grant
	cID := clientID
	cSecret := clientSecret
	if cID == "" {
		// Legacy: subject/secret used as client credentials when clientID not set
		cID = subject
		cSecret = secret
	}
	return &oauth2Auth{
		tokenURL:     tokenURL,
		grantType:    "client_credentials",
		clientID:     cID,
		clientSecret: cSecret,
	}
}

// NewAuthenticatorFromConfig creates an authenticator from run config fields
// without needing the full OpenAPI spec.
func NewAuthenticatorFromConfig(authType, tokenURL, subject, secret, clientID, clientSecret string) Authenticator {
	if subject == "" && secret == "" && clientID == "" && clientSecret == "" {
		return &noneAuth{}
	}

	switch authType {
	case "oauth2", "oidc":
		if tokenURL != "" {
			return newOAuth2Auth(tokenURL, subject, secret, clientID, clientSecret)
		}
		// Fall through to default if no token URL
	case "basic":
		return &basicAuth{username: subject, password: secret}
	case "bearer":
		return &bearerAuth{token: secret}
	case "none", "":
		return &noneAuth{}
	}

	// Default to basic auth when credentials are provided but type is unknown
	return &basicAuth{username: subject, password: secret}
}

// --- No Auth ---

type noneAuth struct{}

func (a *noneAuth) Authenticate(_ *http.Request) error { return nil }
func (a *noneAuth) Name() string                       { return "none" }

// --- Basic Auth ---

type basicAuth struct {
	username string
	password string
}

func (a *basicAuth) Authenticate(req *http.Request) error {
	creds := base64.StdEncoding.EncodeToString([]byte(a.username + ":" + a.password))
	req.Header.Set("Authorization", "Basic "+creds)
	return nil
}

func (a *basicAuth) Name() string { return "basic" }

// --- Bearer Token Auth ---

type bearerAuth struct {
	token string
}

func (a *bearerAuth) Authenticate(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.token)
	return nil
}

func (a *bearerAuth) Name() string { return "bearer" }

// --- OAuth2 Auth ---

type oauth2Auth struct {
	tokenURL     string
	grantType    string // "client_credentials" or "password"
	clientID     string
	clientSecret string
	username     string // only for password grant
	password     string // only for password grant

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func (a *oauth2Auth) Authenticate(req *http.Request) error {
	token, err := a.getToken()
	if err != nil {
		return fmt.Errorf("oauth2 token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

func (a *oauth2Auth) Name() string { return "oauth2" }

func (a *oauth2Auth) getToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.token != "" && time.Now().Before(a.expiresAt) {
		return a.token, nil
	}

	// Try Basic Auth on the token endpoint first (Keycloak and many servers require this),
	// then fall back to sending credentials as form body parameters.
	token, err := a.requestToken(true)
	if err != nil {
		token, err = a.requestToken(false)
	}
	if err != nil {
		return "", err
	}

	a.token = token.AccessToken
	if token.ExpiresIn > 0 {
		a.expiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	} else {
		a.expiresAt = time.Now().Add(1 * time.Hour)
	}

	return a.token, nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (a *oauth2Auth) requestToken(useBasicAuth bool) (*tokenResponse, error) {
	data := url.Values{
		"grant_type": {a.grantType},
	}

	if a.grantType == "password" {
		data.Set("username", a.username)
		data.Set("password", a.password)
	}

	if !useBasicAuth {
		data.Set("client_id", a.clientID)
		if a.clientSecret != "" {
			data.Set("client_secret", a.clientSecret)
		}
	}

	req, err := http.NewRequest("POST", a.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if useBasicAuth && a.clientSecret != "" {
		req.SetBasicAuth(a.clientID, a.clientSecret)
	} else if useBasicAuth {
		// Public client: send client_id as form param, no Basic Auth
		data.Set("client_id", a.clientID)
		req, _ = http.NewRequest("POST", a.tokenURL, strings.NewReader(data.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		detail := string(body)
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, detail)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	return &tokenResp, nil
}

func discoverTokenURL(openIDConnectURL string) (string, error) {
	resp, err := http.Get(openIDConnectURL)
	if err != nil {
		return "", fmt.Errorf("fetching openid discovery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openid discovery returned %d", resp.StatusCode)
	}

	var discovery struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return "", fmt.Errorf("decoding openid discovery: %w", err)
	}
	return discovery.TokenEndpoint, nil
}

func extractTokenURL(scheme openapi.SecurityScheme) string {
	if scheme.Flows == nil {
		return ""
	}
	if scheme.Flows.ClientCredentials != nil && scheme.Flows.ClientCredentials.TokenURL != "" {
		return scheme.Flows.ClientCredentials.TokenURL
	}
	if scheme.Flows.Password != nil && scheme.Flows.Password.TokenURL != "" {
		return scheme.Flows.Password.TokenURL
	}
	if scheme.Flows.AuthorizationCode != nil && scheme.Flows.AuthorizationCode.TokenURL != "" {
		return scheme.Flows.AuthorizationCode.TokenURL
	}
	return ""
}
