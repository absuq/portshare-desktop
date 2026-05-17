package clash

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultDelayTestURL = "https://www.gstatic.com/generate_204"
const defaultDelayTimeoutMS = 5000

type ControllerClient interface {
	Version(context.Context) (Version, error)
	Proxies(context.Context) (ProxySnapshot, error)
	Delay(context.Context, string, string, int) (time.Duration, error)
	Select(context.Context, string, string) error
}

type httpController struct {
	baseURL string
	secret  string
	client  *http.Client
}

func NewHTTPController(baseURL string, secret string) ControllerClient {
	return &httpController{baseURL: strings.TrimRight(baseURL, "/"), secret: secret, client: http.DefaultClient}
}

func (c *httpController) Version(ctx context.Context) (Version, error) {
	var response struct {
		Version string `json:"version"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/version", nil, &response); err != nil {
		return Version{}, err
	}
	return Version{Version: response.Version}, nil
}

func (c *httpController) Proxies(ctx context.Context) (ProxySnapshot, error) {
	var response proxiesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/proxies", nil, &response); err != nil {
		return ProxySnapshot{}, err
	}
	return response.snapshot(), nil
}

func (c *httpController) Delay(ctx context.Context, proxyName string, testURL string, timeoutMS int) (time.Duration, error) {
	if testURL == "" {
		testURL = defaultDelayTestURL
	}
	if timeoutMS <= 0 {
		timeoutMS = defaultDelayTimeoutMS
	}
	path := "/proxies/" + url.PathEscape(proxyName) + "/delay"
	query := url.Values{}
	query.Set("url", testURL)
	query.Set("timeout", fmt.Sprintf("%d", timeoutMS))
	var response struct {
		Delay int `json:"delay"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path+"?"+query.Encode(), nil, &response); err != nil {
		return 0, err
	}
	return time.Duration(response.Delay) * time.Millisecond, nil
}

func (c *httpController) Select(ctx context.Context, groupName string, proxyName string) error {
	payload := map[string]string{"name": proxyName}
	return c.doJSON(ctx, http.MethodPut, "/proxies/"+url.PathEscape(groupName), payload, nil)
}

func (c *httpController) doJSON(ctx context.Context, method string, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.secret != "" {
		request.Header.Set("Authorization", "Bearer "+c.secret)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Clash API 返回 %s", response.Status)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}

type proxyPayload struct {
	Type    string        `json:"type"`
	Now     string        `json:"now"`
	All     []string      `json:"all"`
	History []delayRecord `json:"history"`
}

type delayRecord struct {
	Delay int `json:"delay"`
}

type proxiesResponse struct {
	Proxies map[string]proxyPayload `json:"proxies"`
}

func (r proxiesResponse) snapshot() ProxySnapshot {
	var groups []ProxyGroup
	for name, payload := range r.Proxies {
		if len(payload.All) == 0 {
			continue
		}
		group := ProxyGroup{Name: name, Type: payload.Type, Now: payload.Now}
		for _, optionName := range payload.All {
			option := r.Proxies[optionName]
			group.Options = append(group.Options, ProxyOption{
				Name:  optionName,
				Type:  option.Type,
				Delay: latestDelay(option.History),
			})
		}
		groups = append(groups, group)
	}
	return ProxySnapshot{Groups: groups}
}

func latestDelay(history []delayRecord) time.Duration {
	if len(history) == 0 {
		return 0
	}
	delay := history[len(history)-1].Delay
	if delay <= 0 {
		return 0
	}
	return time.Duration(delay) * time.Millisecond
}

type pipeTransport interface {
	RoundTrip(context.Context, []byte) ([]byte, error)
}

type pipeController struct {
	pipePath  string
	secret    string
	transport pipeTransport
}

func NewPipeController(pipePath string, secret string) ControllerClient {
	return newPipeControllerWithTransport(pipePath, secret, systemPipeTransport{pipePath: pipePath})
}

func newPipeControllerWithTransport(pipePath string, secret string, transport pipeTransport) ControllerClient {
	return &pipeController{pipePath: pipePath, secret: secret, transport: transport}
}

func (c *pipeController) Version(ctx context.Context) (Version, error) {
	var response struct {
		Version string `json:"version"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/version", nil, &response); err != nil {
		return Version{}, err
	}
	return Version{Version: response.Version}, nil
}

func (c *pipeController) Proxies(ctx context.Context) (ProxySnapshot, error) {
	var response proxiesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/proxies", nil, &response); err != nil {
		return ProxySnapshot{}, err
	}
	return response.snapshot(), nil
}

func (c *pipeController) Delay(ctx context.Context, proxyName string, testURL string, timeoutMS int) (time.Duration, error) {
	if testURL == "" {
		testURL = defaultDelayTestURL
	}
	if timeoutMS <= 0 {
		timeoutMS = defaultDelayTimeoutMS
	}
	path := "/proxies/" + url.PathEscape(proxyName) + "/delay"
	query := url.Values{}
	query.Set("url", testURL)
	query.Set("timeout", fmt.Sprintf("%d", timeoutMS))
	var response struct {
		Delay int `json:"delay"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path+"?"+query.Encode(), nil, &response); err != nil {
		return 0, err
	}
	return time.Duration(response.Delay) * time.Millisecond, nil
}

func (c *pipeController) Select(ctx context.Context, groupName string, proxyName string) error {
	payload := map[string]string{"name": proxyName}
	return c.doJSON(ctx, http.MethodPut, "/proxies/"+url.PathEscape(groupName), payload, nil)
}

func (c *pipeController) doJSON(ctx context.Context, method string, path string, payload any, out any) error {
	rawRequest, err := c.rawRequest(method, path, payload)
	if err != nil {
		return err
	}
	rawResponse, err := c.transport.RoundTrip(ctx, rawRequest)
	if err != nil {
		return err
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(rawResponse)), nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Clash pipe API 返回 %s", response.Status)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}

func (c *pipeController) rawRequest(method string, path string, payload any) ([]byte, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	var builder strings.Builder
	builder.WriteString(method + " " + path + " HTTP/1.1\r\n")
	builder.WriteString("Host: mihomo\r\n")
	builder.WriteString("Connection: close\r\n")
	if c.secret != "" {
		builder.WriteString("Authorization: Bearer " + c.secret + "\r\n")
	}
	if payload != nil {
		builder.WriteString("Content-Type: application/json\r\n")
		builder.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
	}
	builder.WriteString("\r\n")
	builder.Write(body)
	return []byte(builder.String()), nil
}
