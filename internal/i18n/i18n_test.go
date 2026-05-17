package i18n

import (
	"testing"

	"github.com/absuq/portshare-desktop/internal/domain"
)

func TestAppTitleIsPortshareInAllLanguages(t *testing.T) {
	zh := NewCatalog(domain.LanguageChinese)
	if got := zh.T(KeyAppTitle); got != "portshare" {
		t.Fatalf("expected Chinese app title to stay portshare, got %q", got)
	}
	en := NewCatalog(domain.LanguageEnglish)
	if got := en.T(KeyAppTitle); got != "portshare" {
		t.Fatalf("expected English app title to stay portshare, got %q", got)
	}
}

func TestDefaultChineseStrings(t *testing.T) {
	c := NewCatalog(domain.LanguageChinese)
	if got := c.T(KeyAddService); got != "添加服务" {
		t.Fatalf("expected Chinese add service text, got %q", got)
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
	if got := c.T(KeyServices); got != "服务" {
		t.Fatalf("expected empty language to default to Chinese services text, got %q", got)
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
