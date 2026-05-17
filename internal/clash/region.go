package clash

import "strings"

func InferRegion(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(name, "上海") || strings.Contains(name, "沪"):
		return "上海"
	case strings.Contains(name, "杭州") || strings.Contains(name, "杭"):
		return "杭州"
	case strings.Contains(name, "香港") || strings.Contains(value, "hk"):
		return "香港"
	case strings.Contains(name, "日本") || strings.Contains(name, "东京") || strings.Contains(value, "jp") || strings.Contains(value, "tokyo"):
		return "日本/东京"
	case strings.Contains(name, "台湾") || strings.Contains(value, "tw"):
		return "台湾"
	case strings.Contains(name, "新加坡") || strings.Contains(value, "sg") || strings.Contains(value, "singapore"):
		return "新加坡"
	default:
		return "未知地区"
	}
}
