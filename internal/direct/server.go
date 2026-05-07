package direct

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
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
	config      ServerConfig
	closed      chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	once        sync.Once
	mu          sync.Mutex
	listener    net.Listener
	active      map[net.Conn]struct{}
	closedState bool
}

func NewServer(config ServerConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		config: config,
		closed: make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
		active: make(map[net.Conn]struct{}),
	}
}

func (s *Server) Serve(listener net.Listener) error {
	s.mu.Lock()
	if s.closedState {
		s.mu.Unlock()
		_ = listener.Close()
		return nil
	}
	s.listener = listener
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
		if !s.addActive(conn) {
			_ = conn.Close()
			continue
		}
		go s.handle(conn)
	}
}

func (s *Server) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		s.closedState = true
		listener := s.listener
		active := make([]net.Conn, 0, len(s.active))
		for conn := range s.active {
			active = append(active, conn)
		}
		s.mu.Unlock()
		s.cancel()
		close(s.closed)
		if listener != nil {
			_ = listener.Close()
		}
		for _, conn := range active {
			_ = conn.Close()
		}
	})
	return nil
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	defer s.removeActive(conn)
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

	var next protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &next); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return
		}
		return
	}
	if next.Type != protocol.TypeOpenTCP || next.Version != protocol.Version {
		_ = writeOpenTCPError(conn, fmt.Sprintf("unexpected message: %s", next.Type))
		return
	}
	s.handleOpenTCP(conn, next)
}

func (s *Server) handleOpenTCP(conn net.Conn, msg protocol.ControlMessage) {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	target, err := dialer.DialContext(s.ctx, "tcp", net.JoinHostPort(msg.TargetHost, strconv.Itoa(msg.TargetPort)))
	if err != nil {
		_ = writeOpenTCPError(conn, err.Error())
		return
	}
	if !s.addActive(target) {
		_ = target.Close()
		return
	}
	defer s.removeActive(target)

	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:    protocol.TypeOpenTCPOK,
		Version: protocol.Version,
	}); err != nil {
		return
	}

	_ = conn.SetDeadline(time.Time{})
	_ = target.SetDeadline(time.Time{})

	PipeBidirectional(conn, target)
}

func writeAuthFailure(conn net.Conn) error {
	return writeOpenTCPError(conn, ErrAuthFailed.Error())
}

func writeOpenTCPError(conn net.Conn, text string) error {
	return protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:    protocol.TypeOpenTCPError,
		Version: protocol.Version,
		Error:   text,
	})
}

func (s *Server) addActive(conns ...net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closedState {
		return false
	}
	for _, conn := range conns {
		s.active[conn] = struct{}{}
	}
	return true
}

func (s *Server) removeActive(conns ...net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, conn := range conns {
		delete(s.active, conn)
	}
}
