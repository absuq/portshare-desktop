package tailscale

import (
	"context"
	"strings"
	"testing"

	"github.com/absuq/portshare-desktop/internal/domain"
)

type recordingRunner struct {
	commands []string
	output   string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	return []byte(r.output), nil
}

func TestPublishTailnetUsesServe(t *testing.T) {
	r := &recordingRunner{output: `{"TCP":{}}`}
	p := New(r)
	_, err := p.Publish(context.Background(), domain.LocalService{ID: "svc", Scheme: "http", Host: "127.0.0.1", Port: 3000}, domain.ModeTailnet, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.commands[0], "tailscale serve --bg --http=3000 http://127.0.0.1:3000") {
		t.Fatalf("unexpected command: %+v", r.commands)
	}
}
