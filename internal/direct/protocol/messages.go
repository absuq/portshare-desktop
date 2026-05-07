package protocol

const Version = 1

type MessageType string

const (
	TypeHello        MessageType = "hello"
	TypeHelloResp    MessageType = "hello_response"
	TypeAuthProof    MessageType = "auth_proof"
	TypeAuthOK       MessageType = "auth_ok"
	TypeOpenTCP      MessageType = "open_tcp"
	TypeOpenTCPOK    MessageType = "open_tcp_ok"
	TypeOpenTCPError MessageType = "open_tcp_error"
)

type ControlMessage struct {
	Type       MessageType `json:"type"`
	Version    int         `json:"version"`
	DeviceID   string      `json:"device_id,omitempty"`
	DeviceName string      `json:"device_name,omitempty"`
	Nonce      []byte      `json:"nonce,omitempty"`
	Proof      []byte      `json:"proof,omitempty"`
	TargetHost string      `json:"target_host,omitempty"`
	TargetPort int         `json:"target_port,omitempty"`
	Error      string      `json:"error,omitempty"`
}
