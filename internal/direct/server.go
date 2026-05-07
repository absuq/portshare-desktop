package direct

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/absuq/portshare-desktop/internal/direct/protocol"
)

var ErrAuthFailed = errors.New("authentication failed")

type ServerConfig struct {
	DeviceID   string
	DeviceName string
	Secret     string
}

type Server struct {
	config   ServerConfig
	closed   chan struct{}
	once     sync.Once
	mu       sync.Mutex
	listener net.Listener
}

func NewServer(config ServerConfig) *Server {
	return &Server{
		config: config,
		closed: make(chan struct{}),
	}
}

func (s *Server) Serve(listener net.Listener) error {
	s.mu.Lock()
	select {
	case <-s.closed:
		s.mu.Unlock()
		_ = listener.Close()
		return nil
	default:
		s.listener = listener
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.listener == listener {
			s.listener = nil
		}
		s.mu.Unlock()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return nil
			default:
				return err
			}
		}
		select {
		case <-s.closed:
			_ = conn.Close()
			return nil
		default:
		}
		go s.handle(conn)
	}
}

func (s *Server) Close() error {
	s.once.Do(func() {
		close(s.closed)
		s.mu.Lock()
		listener := s.listener
		s.mu.Unlock()
		if listener != nil {
			_ = listener.Close()
		}
	})
	return nil
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	var hello protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &hello); err != nil {
		return
	}
	if hello.Type != protocol.TypeHello || hello.Version != protocol.Version {
		_ = writeAuthFailure(conn)
		return
	}

	responderNonce, err := protocol.NewNonce()
	if err != nil {
		return
	}
	serverProof := protocol.ComputeProof(s.config.Secret, s.config.DeviceID, hello.DeviceID, hello.Nonce, responderNonce)
	response := protocol.ControlMessage{
		Type:       protocol.TypeHelloResp,
		Version:    protocol.Version,
		DeviceID:   s.config.DeviceID,
		DeviceName: s.config.DeviceName,
		Nonce:      responderNonce,
		Proof:      serverProof,
	}
	if err := protocol.WriteFrame(conn, response); err != nil {
		return
	}

	var authProof protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &authProof); err != nil {
		return
	}
	if authProof.Type != protocol.TypeAuthProof ||
		authProof.Version != protocol.Version ||
		!protocol.VerifyProof(s.config.Secret, hello.DeviceID, s.config.DeviceID, hello.Nonce, responderNonce, authProof.Proof) {
		_ = writeAuthFailure(conn)
		return
	}

	_ = protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:       protocol.TypeAuthOK,
		Version:    protocol.Version,
		DeviceID:   s.config.DeviceID,
		DeviceName: s.config.DeviceName,
	})
}

func writeAuthFailure(conn net.Conn) error {
	return protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:    protocol.TypeOpenTCPError,
		Version: protocol.Version,
		Error:   ErrAuthFailed.Error(),
	})
}
