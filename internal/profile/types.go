package profile

type CRUDType string

const (
	SendNone CRUDType = "SEND_NONE"
	SendOne  CRUDType = "SEND_ONE"
	SendMany CRUDType = "SEND_MANY"
	ReadNone CRUDType = "READ_NONE"
	ReadOne  CRUDType = "READ_ONE"
	ReadMany CRUDType = "READ_MANY"
)

type Operation struct {
	Name           string
	Optional       bool
	Method         string
	Pattern        string
	SendType       CRUDType
	ReadType       CRUDType
	ExpectedStatus int
	Description    string
	Precondition   bool
}

type Profile struct {
	Name           string
	Operations     []Operation
	ExecutionOrder []string
	ParamMappings  map[string]string // paramName -> domain ("$" = current domain)
}
