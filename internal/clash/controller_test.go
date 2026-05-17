package clash

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPControllerCallsVersionProxiesDelayAndSelect(t *testing.T) {
	var sawAuth bool
	var selectedName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer test-secret" {
			sawAuth = true
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/version":
			_, _ = w.Write([]byte(`{"version":"Mihomo 1.18"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/proxies":
			_, _ = w.Write([]byte(`{"proxies":{
				"GLOBAL":{"type":"Selector","now":"上海 01","all":["上海 01","杭州 01"]},
				"上海 01":{"type":"Shadowsocks","history":[{"delay":23}]},
				"杭州 01":{"type":"Vmess","history":[{"delay":35}]}
			}}`))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/proxies/%E4%B8%8A%E6%B5%B7%2001/delay":
			if r.URL.Query().Get("timeout") != "5000" {
				t.Fatalf("unexpected timeout query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"delay":23}`))
		case r.Method == http.MethodPut && r.URL.Path == "/proxies/GLOBAL":
			var payload struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			selectedName = payload.Name
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewHTTPController(server.URL, "test-secret")
	version, err := client.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version.Version != "Mihomo 1.18" {
		t.Fatalf("unexpected version: %+v", version)
	}
	snapshot, err := client.Proxies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Groups) != 1 || snapshot.Groups[0].Now != "上海 01" || len(snapshot.Groups[0].Options) != 2 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	delay, err := client.Delay(context.Background(), "上海 01", "https://www.gstatic.com/generate_204", 5000)
	if err != nil {
		t.Fatal(err)
	}
	if delay != 23*time.Millisecond {
		t.Fatalf("unexpected delay: %s", delay)
	}
	if err := client.Select(context.Background(), "GLOBAL", "上海 01"); err != nil {
		t.Fatal(err)
	}
	if selectedName != "上海 01" {
		t.Fatalf("unexpected selected node: %q", selectedName)
	}
	if !sawAuth {
		t.Fatal("expected Authorization header")
	}
}

func TestPipeControllerFormatsHTTPRequest(t *testing.T) {
	transport := &fakePipeTransport{}
	client := newPipeControllerWithTransport(`\\.\pipe\verge-mihomo`, "secret", transport)

	_, err := client.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	request := transport.lastRequest
	if !strings.Contains(request, "GET /version HTTP/1.1") {
		t.Fatalf("expected version request, got %q", request)
	}
	if !strings.Contains(request, "Authorization: Bearer secret") {
		t.Fatalf("expected authorization header, got %q", request)
	}
}

type fakePipeTransport struct {
	lastRequest string
}

func (f *fakePipeTransport) RoundTrip(_ context.Context, request []byte) ([]byte, error) {
	f.lastRequest = string(request)
	return []byte("HTTP/1.1 200 OK\r\nContent-Length: 20\r\n\r\n{\"version\":\"pipe\"}"), nil
}
