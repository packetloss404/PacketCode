package mcp

import "encoding/json"

// JSONRPCVersion is the only JSON-RPC version this package speaks.
const JSONRPCVersion = "2.0"

// Standard JSON-RPC 2.0 error codes referenced by this package.
const (
	// ErrCodeMethodNotFound is the JSON-RPC 2.0 sentinel for an unknown
	// method. We return this when the server initiates a request whose
	// method we deliberately do not implement (sampling, roots, etc.).
	ErrCodeMethodNotFound = -32601
	// ErrCodeInternalError is the JSON-RPC 2.0 sentinel for an internal
	// failure. Reserved for future use; not produced by this package
	// today but exposed so callers can refer to it by name.
	ErrCodeInternalError = -32603
)

// Request is a JSON-RPC 2.0 request envelope. Notifications are encoded
// via Notification; this struct always carries an id.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response envelope. Exactly one of Result or
// Error will be non-nil on a well-formed message.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObj       `json:"error,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification envelope. No id and no
// response is expected.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ErrorObj is the standard JSON-RPC 2.0 error object.
type ErrorObj struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface so an *ErrorObj can flow back to
// callers as a regular Go error.
func (e *ErrorObj) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// newRequest builds a Request with the JSON-RPC version pre-filled and
// params marshalled (or left empty if params is nil). It panics on
// marshal failure since the caller passes Go-defined values.
func newRequest(id int64, method string, params any) Request {
	r := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(fmtInt64(id)),
		Method:  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			panic("mcp: marshal request params: " + err.Error())
		}
		r.Params = raw
	}
	return r
}

func fmtInt64(id int64) []byte {
	if id == 0 {
		return []byte("0")
	}
	neg := id < 0
	if neg {
		id = -id
	}
	var buf [20]byte
	pos := len(buf)
	for id > 0 {
		pos--
		buf[pos] = byte('0' + id%10)
		id /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return buf[pos:]
}

// newNotification builds a Notification with the JSON-RPC version
// pre-filled. Same panic semantics as newRequest.
func newNotification(method string, params any) Notification {
	n := Notification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			panic("mcp: marshal notification params: " + err.Error())
		}
		n.Params = raw
	}
	return n
}
