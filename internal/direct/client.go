package direct

import (
	"context"
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

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return zero, err
	}
	defer conn.Close()
	stopCancelWatcher := make(chan struct{})
	defer close(stopCancelWatcher)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stopCancelWatcher:
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

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	initiatorNonce, err := protocol.NewNonce()
	if err != nil {
		return zero, err
	}
	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:       protocol.TypeHello,
		Version:    protocol.Version,
		DeviceID:   c.config.DeviceID,
		DeviceName: c.config.DeviceName,
		Nonce:      initiatorNonce,
	}); err != nil {
		return zero, contextErr(err)
	}

	var response protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &response); err != nil {
		return zero, contextErr(err)
	}
	if response.Type != protocol.TypeHelloResp ||
		response.Version != protocol.Version ||
		!protocol.VerifyProof(c.config.Secret, response.DeviceID, c.config.DeviceID, initiatorNonce, response.Nonce, response.Proof) {
		return zero, ErrAuthFailed
	}

	clientProof := protocol.ComputeProof(c.config.Secret, c.config.DeviceID, response.DeviceID, initiatorNonce, response.Nonce)
	if err := protocol.WriteFrame(conn, protocol.ControlMessage{
		Type:    protocol.TypeAuthProof,
		Version: protocol.Version,
		Proof:   clientProof,
	}); err != nil {
		return zero, contextErr(err)
	}

	var authOK protocol.ControlMessage
	if err := protocol.ReadFrame(conn, &authOK); err != nil {
		return zero, contextErr(err)
	}
	if authOK.Type != protocol.TypeAuthOK || authOK.Version != protocol.Version {
		return zero, ErrAuthFailed
	}

	return PairedPeer{
		DeviceID:   response.DeviceID,
		DeviceName: response.DeviceName,
		Address:    address,
	}, nil
}
