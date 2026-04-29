package fake

import (
	"context"
	"testing"

	"github.com/absuq/portshare-desktop/internal/domain"
)

func TestPublishAndStop(t *testing.T) {
	p := New("fake")
	share, err := p.Publish(context.Background(), domain.LocalService{ID: "svc", Scheme: "http", Host: "127.0.0.1", Port: 3000}, domain.ModeTailnet, nil)
	if err != nil {
		t.Fatal(err)
	}
	if share.Status != domain.ShareActive || share.PublicURL == "" {
		t.Fatalf("unexpected share: %+v", share)
	}
	if err := p.Stop(context.Background(), share.ID); err != nil {
		t.Fatal(err)
	}
	shares, err := p.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 0 {
		t.Fatalf("expected no shares, got %+v", shares)
	}
}
