package direct

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/absuq/portshare-desktop/internal/direct/protocol"
)

func TestPairingSucceedsWithMatchingSecret(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	server := NewServer(ServerConfig{
		DeviceID:   "device-b",
		DeviceName: "desktop-b",
		Secret:     "shared",
	})
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	client := NewClient(ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	peer, err := client.Pair(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if peer.DeviceID != "device-b" || peer.DeviceName != "desktop-b" {
		t.Fatalf("unexpected peer: %+v", peer)
	}
}

func TestPairingFailsWithWrongSecret(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	server := NewServer(ServerConfig{DeviceID: "device-b", DeviceName: "desktop-b", Secret: "right"})
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	client := NewClient(ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "wrong"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.Pair(ctx, listener.Addr().String()); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
}

func TestServerCloseStopsServe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(ServerConfig{DeviceID: "device-b", DeviceName: "desktop-b", Secret: "shared"})
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected Serve to stop cleanly, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected Serve to return after Close")
	}
}

func TestServerRejectsConnectionsAfterClose(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()

	server := NewServer(ServerConfig{DeviceID: "device-b", DeviceName: "desktop-b", Secret: "shared"})
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected Serve to return after Close")
	}

	client := NewClient(ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.Pair(ctx, address); err == nil {
		t.Fatal("expected pairing to fail after server close")
	}
}

func TestPairReturnsContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	client := NewClient(ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.Pair(ctx, listener.Addr().String())
		done <- err
	}()

	select {
	case conn := <-accepted:
		defer conn.Close()
	case <-time.After(time.Second):
		t.Fatal("server did not accept connection")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected Pair to return quickly after context cancellation")
	}
}

func TestPairRejectsHelloResponseVersionMismatch(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var hello protocol.ControlMessage
		if err := protocol.ReadFrame(conn, &hello); err != nil {
			return
		}
		responderNonce, err := protocol.NewNonce()
		if err != nil {
			return
		}
		_ = protocol.WriteFrame(conn, protocol.ControlMessage{
			Type:     protocol.TypeHelloResp,
			Version:  protocol.Version + 1,
			DeviceID: "device-b",
			Nonce:    responderNonce,
			Proof:    protocol.ComputeProof("shared", "device-b", hello.DeviceID, hello.Nonce, responderNonce),
		})
	}()

	client := NewClient(ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.Pair(ctx, listener.Addr().String()); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
}

func TestPairRejectsAuthOKVersionMismatch(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var hello protocol.ControlMessage
		if err := protocol.ReadFrame(conn, &hello); err != nil {
			return
		}
		responderNonce, err := protocol.NewNonce()
		if err != nil {
			return
		}
		if err := protocol.WriteFrame(conn, protocol.ControlMessage{
			Type:     protocol.TypeHelloResp,
			Version:  protocol.Version,
			DeviceID: "device-b",
			Nonce:    responderNonce,
			Proof:    protocol.ComputeProof("shared", "device-b", hello.DeviceID, hello.Nonce, responderNonce),
		}); err != nil {
			return
		}
		var authProof protocol.ControlMessage
		if err := protocol.ReadFrame(conn, &authProof); err != nil {
			return
		}
		_ = protocol.WriteFrame(conn, protocol.ControlMessage{
			Type:    protocol.TypeAuthOK,
			Version: protocol.Version + 1,
		})
	}()

	client := NewClient(ClientConfig{DeviceID: "device-a", DeviceName: "desktop-a", Secret: "shared"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.Pair(ctx, listener.Addr().String()); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
}

func TestServerRejectsAuthProofVersionMismatch(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	server := NewServer(ServerConfig{DeviceID: "device-b", DeviceName: "desktop-b", Secret: "shared"})
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	initiatorNonce, err := protocol.NewNonce()
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:     protocol.TypeHello,
		Version:  protocol.Version,
		DeviceID: "device-a",
		Nonce:    initiatorNonce,
	}); err != nil {
		t.Fatal(err)
	}

	var response protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &response); err != nil {
		t.Fatal(err)
	}
	proof := protocol.ComputeProof("shared", "device-a", response.DeviceID, initiatorNonce, response.Nonce)
	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:    protocol.TypeAuthProof,
		Version: protocol.Version + 1,
		Proof:   proof,
	}); err != nil {
		t.Fatal(err)
	}

	var failure protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Type != protocol.TypeOpenTCPError || failure.Error != ErrAuthFailed.Error() {
		t.Fatalf("expected auth failure, got %+v", failure)
	}
}

func TestServerAuthOKIncludesDeviceMetadata(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	server := NewServer(ServerConfig{DeviceID: "device-b", DeviceName: "desktop-b", Secret: "shared"})
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	initiatorNonce, err := protocol.NewNonce()
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:     protocol.TypeHello,
		Version:  protocol.Version,
		DeviceID: "device-a",
		Nonce:    initiatorNonce,
	}); err != nil {
		t.Fatal(err)
	}

	var response protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &response); err != nil {
		t.Fatal(err)
	}
	proof := protocol.ComputeProof("shared", "device-a", response.DeviceID, initiatorNonce, response.Nonce)
	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:    protocol.TypeAuthProof,
		Version: protocol.Version,
		Proof:   proof,
	}); err != nil {
		t.Fatal(err)
	}

	var authOK protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &authOK); err != nil {
		t.Fatal(err)
	}
	if authOK.Type != protocol.TypeAuthOK ||
		authOK.Version != protocol.Version ||
		authOK.DeviceID != "device-b" ||
		authOK.DeviceName != "desktop-b" {
		t.Fatalf("unexpected auth_ok: %+v", authOK)
	}
}
