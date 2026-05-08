package localhostbridge

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"
)

func TestBridgeForwardsTCPToLoopbackTarget(t *testing.T) {
	target, err := startLineEchoServer("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	bridge := NewBridge(BridgePlan{
		ListenAddress:  "127.0.0.1:0",
		TargetAddress:  target.Addr().String(),
		AllowedPeerIPs: []string{"127.0.0.1"},
	})
	if err := bridge.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	conn, err := net.DialTimeout("tcp", bridge.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if line != "hello\n" {
		t.Fatalf("unexpected echo response: %q", line)
	}
}

func TestBridgeRejectsUntrustedRemoteIP(t *testing.T) {
	target, err := startLineEchoServer("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	bridge := NewBridge(BridgePlan{
		ListenAddress:  "127.0.0.1:0",
		TargetAddress:  target.Addr().String(),
		AllowedPeerIPs: []string{"100.109.251.97"},
	})
	if err := bridge.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	conn, err := net.DialTimeout("tcp", bridge.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := conn.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(conn).ReadString('\n'); err == nil {
		t.Fatal("expected untrusted connection to be closed")
	}
}

func startLineEchoServer(address string) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				_, _ = conn.Write([]byte(line))
			}(conn)
		}
	}()
	return listener, nil
}
