package protocol

const Version = 1

type MessageType string

const (
	TypeHello     MessageType = "hello"
	TypeHelloResp MessageType = "hello_response"
	TypeAuthProof MessageType = "auth_proof"
	TypeAuthOK    MessageType = "auth_ok"
	TypeAuthError MessageType = "auth_error"
)

type ControlMessage struct {
	Type       MessageType `json:"type"`
	Version    int         `json:"version"`
	DeviceID   string      `json:"device_id,omitempty"`
	DeviceName string      `json:"device_name,omitempty"`
	Nonce      []byte      `json:"nonce,omitempty"`
	Proof      []byte      `json:"proof,omitempty"`
	Error      string      `json:"error,omitempty"`
}
