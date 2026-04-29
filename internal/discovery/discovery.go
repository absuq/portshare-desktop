package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
)

var titlePattern = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func Probe(rawURL string, timeout time.Duration) (domain.LocalService, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return domain.LocalService{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return domain.LocalService{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return domain.LocalService{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	host := parsed.Hostname()
	port, _ := strconv.Atoi(parsed.Port())
	title := extractTitle(string(body))
	if title == "" {
		title = fmt.Sprintf("本地服务 %d", port)
	}
	return domain.LocalService{
		ID:          fmt.Sprintf("%s-%s-%d", parsed.Scheme, host, port),
		Name:        title,
		Scheme:      parsed.Scheme,
		Host:        host,
		Port:        port,
		Title:       title,
		Discovered:  true,
		LastChecked: time.Now(),
	}, nil
}

func extractTitle(html string) string {
	matches := titlePattern.FindStringSubmatch(html)
	if len(matches) < 2 {
		return ""
	}
	title := strings.TrimSpace(matches[1])
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\t", " ")
	return title
}

func ScanCommon(timeout time.Duration) []domain.LocalService {
	ports := []int{3000, 5173, 8080, 8000, 5000, 4200, 8443}
	var services []domain.LocalService
	for _, port := range ports {
		for _, scheme := range []string{"http", "https"} {
			svc, err := Probe(fmt.Sprintf("%s://127.0.0.1:%d", scheme, port), timeout)
			if err == nil {
				services = append(services, svc)
				break
			}
		}
	}
	return services
}
