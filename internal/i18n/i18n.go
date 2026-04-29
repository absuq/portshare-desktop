package i18n

import "github.com/absuq/portshare-desktop/internal/domain"

type Key string

const (
	KeyAppTitle       Key = "app.title"
	KeyServices       Key = "services"
	KeyHistory        Key = "history"
	KeySettings       Key = "settings"
	KeyAddService     Key = "add.service"
	KeyRefresh        Key = "refresh"
	KeyTailnetShare   Key = "tailnet.share"
	KeyPublicShare    Key = "public.share"
	KeyStopShare      Key = "stop.share"
	KeyStopAll        Key = "stop.all"
	KeyPausePublic    Key = "pause.public"
	KeyPublicWarning  Key = "public.warning"
	KeyLongRunWarning Key = "longrun.warning"
)

type Catalog struct {
	lang domain.Language
}

func NewCatalog(lang domain.Language) Catalog {
	if lang == "" {
		lang = domain.LanguageChinese
	}
	return Catalog{lang: lang}
}

func (c Catalog) T(key Key) string {
	if c.lang == domain.LanguageEnglish {
		if v, ok := en[key]; ok {
			return v
		}
	}
	if v, ok := zh[key]; ok {
		return v
	}
	return string(key)
}

var zh = map[Key]string{
	KeyAppTitle:       "端口发布器",
	KeyServices:       "服务",
	KeyHistory:        "历史",
	KeySettings:       "设置",
	KeyAddService:     "添加服务",
	KeyRefresh:        "刷新发现",
	KeyTailnetShare:   "开放到 tailnet",
	KeyPublicShare:    "开启公网",
	KeyStopShare:      "关闭发布",
	KeyStopAll:        "停止全部发布",
	KeyPausePublic:    "暂停所有公网",
	KeyPublicWarning:  "公网开放会让非 tailnet 设备访问该服务，请确认服务本身已有保护。",
	KeyLongRunWarning: "长期开放不会自动关闭，请确认你愿意持续暴露该公网入口。",
}

var en = map[Key]string{
	KeyAppTitle:       "PortShare",
	KeyServices:       "Services",
	KeyHistory:        "History",
	KeySettings:       "Settings",
	KeyAddService:     "Add service",
	KeyRefresh:        "Refresh discovery",
	KeyTailnetShare:   "Share to tailnet",
	KeyPublicShare:    "Public share",
	KeyStopShare:      "Stop share",
	KeyStopAll:        "Stop all shares",
	KeyPausePublic:    "Pause public shares",
	KeyPublicWarning:  "Public sharing allows non-tailnet devices to access this service. Confirm the service is protected.",
	KeyLongRunWarning: "Long-term public sharing does not close automatically. Confirm you want to keep this public entry open.",
}
