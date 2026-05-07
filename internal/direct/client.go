package direct

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/absuq/portshare-desktop/internal/direct/protocol"
)

type ClientConfig struct {
	DeviceID   string
	DeviceName string
	Secret     string
}

type Client struct {
	config ClientConfig
}

func NewClient(config ClientConfig) Client {
	return Client{config: config}
}

type PairedPeer struct {
	DeviceID   string
	DeviceName string
	Address    string
}

func (c Client) Pair(ctx context.Context, address string) (PairedPeer, error) {
	var zero PairedPeer

	conn, peer, stopCancelWatcher, err := c.authenticate(ctx, address)
	if err != nil {
		return zero, err
	}
	defer conn.Close()
	defer stopCancelWatcher()

	return peer, nil
}

func (c Client) OpenTCP(ctx context.Context, peerAddress, targetHost string, targetPort int) (net.Conn, error) {
	conn, _, stopCancelWatcher, err := c.authenticate(ctx, peerAddress)
	if err != nil {
		return nil, err
	}

	closeOnError := true
	defer func() {
		stopCancelWatcher()
		if closeOnError {
			_ = conn.Close()
		}
	}()

	contextErr := func(err error) error {
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}

	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:       protocol.TypeOpenTCP,
		Version:    protocol.Version,
		TargetHost: targetHost,
		TargetPort: targetPort,
	}); err != nil {
		return nil, contextErr(err)
	}

	var response protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &response); err != nil {
		return nil, contextErr(err)
	}
	switch {
	case response.Type == protocol.TypeOpenTCPOK && response.Version == protocol.Version:
		_ = conn.SetDeadline(time.Time{})
		closeOnError = false
		return conn, nil
	case response.Type == protocol.TypeOpenTCPError && response.Error != "":
		return nil, errors.New(response.Error)
	default:
		return nil, fmt.Errorf("unexpected open_tcp response: %s", response.Type)
	}
}

func (c Client) authenticate(ctx context.Context, address string) (net.Conn, PairedPeer, func(), error) {
	var zero PairedPeer

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, zero, func() {}, err
	}
	stopCancelWatcher := make(chan struct{})
	stopOnce := make(chan struct{})
	go func() {
		defer close(stopOnce)
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stopCancelWatcher:
		}
	}()
	stop := func() {
		close(stopCancelWatcher)
		<-stopOnce
	}

	contextErr := func(err error) error {
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	initiatorNonce, err := protocol.NewNonce()
	if err != nil {
		stop()
		_ = conn.Close()
		return nil, zero, func() {}, err
	}
	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:       protocol.TypeHello,
		Version:    protocol.Version,
		DeviceID:   c.config.DeviceID,
		DeviceName: c.config.DeviceName,
		Nonce:      initiatorNonce,
	}); err != nil {
		stop()
		_ = conn.Close()
		return nil, zero, func() {}, contextErr(err)
	}

	var response protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &response); err != nil {
		stop()
		_ = conn.Close()
		return nil, zero, func() {}, contextErr(err)
	}
	if response.Type != protocol.TypeHelloResp ||
		response.Version != protocol.Version ||
		!protocol.VerifyProof(c.config.Secret, response.DeviceID, c.config.DeviceID, initiatorNonce, response.Nonce, response.Proof) {
		stop()
		_ = conn.Close()
		return nil, zero, func() {}, ErrAuthFailed
	}

	clientProof := protocol.ComputeProof(c.config.Secret, c.config.DeviceID, response.DeviceID, initiatorNonce, response.Nonce)
	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:    protocol.TypeAuthProof,
		Version: protocol.Version,
		Proof:   clientProof,
	}); err != nil {
		stop()
		_ = conn.Close()
		return nil, zero, func() {}, contextErr(err)
	}

	var authOK protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &authOK); err != nil {
		stop()
		_ = conn.Close()
		return nil, zero, func() {}, contextErr(err)
	}
	if authOK.Type != protocol.TypeAuthOK || authOK.Version != protocol.Version {
		stop()
		_ = conn.Close()
		return nil, zero, func() {}, ErrAuthFailed
	}

	return conn, PairedPeer{
		DeviceID:   response.DeviceID,
		DeviceName: response.DeviceName,
		Address:    address,
	}, stop, nil
}
