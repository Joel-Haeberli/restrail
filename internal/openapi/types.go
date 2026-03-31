package openapi

import "encoding/json"

type Spec struct {
	OpenAPI    string                `json:"openapi"`
	Info       Info                  `json:"info"`
	Paths      map[string]*PathItem `json:"paths"`
	Components Components            `json:"components"`
	Security   []SecurityRequirement `json:"security"`
}

type Info struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type PathItem struct {
	Get     *Operation `json:"get"`
	Post    *Operation `json:"post"`
	Put     *Operation `json:"put"`
	Delete  *Operation `json:"delete"`
	Patch   *Operation `json:"patch"`
	Options *Operation `json:"options"`
	Head    *Operation `json:"head"`
}

func (p *PathItem) MethodOperation(method string) *Operation {
	switch method {
	case "GET":
		return p.Get
	case "POST":
		return p.Post
	case "PUT":
		return p.Put
	case "DELETE":
		return p.Delete
	case "PATCH":
		return p.Patch
	case "OPTIONS":
		return p.Options
	case "HEAD":
		return p.Head
	}
	return nil
}

type Operation struct {
	OperationID string                `json:"operationId"`
	Summary     string                `json:"summary"`
	Description string                `json:"description"`
	Parameters  []Parameter           `json:"parameters"`
	RequestBody *RequestBody          `json:"requestBody"`
	Responses   map[string]*Response  `json:"responses"`
	Security    []SecurityRequirement `json:"security"`
	Tags        []string              `json:"tags"`
}

type Parameter struct {
	Name     string  `json:"name"`
	In       string  `json:"in"`
	Required bool    `json:"required"`
	Schema   *Schema `json:"schema"`
}

type RequestBody struct {
	Description string               `json:"description"`
	Required    bool                 `json:"required"`
	Content     map[string]MediaType `json:"content"`
}

type MediaType struct {
	Schema *Schema `json:"schema"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content"`
}

type Schema struct {
	Ref         string              `json:"$ref"`
	Type        string              `json:"type"`
	Format      string              `json:"format"`
	Properties  map[string]*Schema  `json:"properties"`
	Items       *Schema             `json:"items"`
	Required    []string            `json:"required"`
	Enum        []json.RawMessage   `json:"enum"`
	AllOf       []*Schema           `json:"allOf"`
	OneOf       []*Schema           `json:"oneOf"`
	AnyOf       []*Schema           `json:"anyOf"`
	Minimum     *float64            `json:"minimum"`
	Maximum     *float64            `json:"maximum"`
	MinLength   *int                `json:"minLength"`
	MaxLength   *int                `json:"maxLength"`
	Description string              `json:"description"`
	Default     json.RawMessage     `json:"default"`
	Example     json.RawMessage     `json:"example"`
	Nullable    bool                `json:"nullable"`
	ReadOnly    bool                `json:"readOnly"`
	WriteOnly   bool                `json:"writeOnly"`
}

type Components struct {
	Schemas         map[string]*Schema         `json:"schemas"`
	SecuritySchemes map[string]SecurityScheme  `json:"securitySchemes"`
}

type SecurityScheme struct {
	Type             string      `json:"type"`
	Scheme           string      `json:"scheme"`
	BearerFormat     string      `json:"bearerFormat"`
	Flows            *OAuthFlows `json:"flows"`
	OpenIDConnectURL string      `json:"openIdConnectUrl"`
}

type OAuthFlows struct {
	ClientCredentials *OAuthFlow `json:"clientCredentials"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode"`
	Implicit          *OAuthFlow `json:"implicit"`
	Password          *OAuthFlow `json:"password"`
}

type OAuthFlow struct {
	TokenURL         string            `json:"tokenUrl"`
	AuthorizationURL string            `json:"authorizationUrl"`
	RefreshURL       string            `json:"refreshUrl"`
	Scopes           map[string]string `json:"scopes"`
}

type SecurityRequirement map[string][]string
