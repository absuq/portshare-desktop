package manager

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/absuq/portshare-desktop/internal/audit"
	"github.com/absuq/portshare-desktop/internal/domain"
	"github.com/absuq/portshare-desktop/internal/provider/fake"
)

func TestPublicShareExpires(t *testing.T) {
	ctx := context.Background()
	p := fake.New("fake")
	log := audit.NewLog(filepath.Join(t.TempDir(), "audit.jsonl"))
	m := New(p, log)
	svc := domain.LocalService{ID: "svc", Scheme: "http", Host: "127.0.0.1", Port: 3000}
	share, err := m.PublishPublic(ctx, svc, 20*time.Millisecond, false)
	if err != nil {
		t.Fatal(err)
	}
	if share.ExpiresAt == nil {
		t.Fatal("expected expiry")
	}
	time.Sleep(80 * time.Millisecond)
	shares, err := p.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 0 {
		t.Fatalf("expected expired share to stop, got %+v", shares)
	}
}
