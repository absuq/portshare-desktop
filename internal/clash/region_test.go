package clash

import "testing"

func TestInferRegion(t *testing.T) {
	tests := map[string]string{
		"上海 01":      "上海",
		"沪-联通":       "上海",
		"杭州-移动":      "杭州",
		"HK Premium": "香港",
		"JP Tokyo":   "日本/东京",
		"plain-node": "未知地区",
	}

	for name, want := range tests {
		if got := InferRegion(name); got != want {
			t.Fatalf("InferRegion(%q) = %q, want %q", name, got, want)
		}
	}
}
