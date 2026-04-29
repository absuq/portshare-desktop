package i18n

import (
	"testing"

	"github.com/absuq/portshare-desktop/internal/domain"
)

func TestDefaultChineseStrings(t *testing.T) {
	c := NewCatalog(domain.LanguageChinese)
	if got := c.T(KeyAppTitle); got != "端口发布器" {
		t.Fatalf("expected Chinese app title, got %q", got)
	}
}

func TestEnglishSwitch(t *testing.T) {
	c := NewCatalog(domain.LanguageEnglish)
	if got := c.T(KeyPublicShare); got != "Public share" {
		t.Fatalf("expected English text, got %q", got)
	}
}

func TestEmptyLanguageDefaultsToChinese(t *testing.T) {
	c := NewCatalog("")
	if got := c.T(KeyAppTitle); got != "端口发布器" {
		t.Fatalf("expected Chinese app title, got %q", got)
	}
}

func TestUnknownKeyReturnsKey(t *testing.T) {
	c := NewCatalog(domain.LanguageChinese)
	if got := c.T(Key("missing.key")); got != "missing.key" {
		t.Fatalf("expected missing key fallback, got %q", got)
	}
}

func TestEnglishMissingKeyFallsBackToChinese(t *testing.T) {
	original, ok := en[KeyServices]
	delete(en, KeyServices)
	defer func() {
		if ok {
			en[KeyServices] = original
		}
	}()

	c := NewCatalog(domain.LanguageEnglish)
	if got := c.T(KeyServices); got != "服务" {
		t.Fatalf("expected Chinese fallback, got %q", got)
	}
}
