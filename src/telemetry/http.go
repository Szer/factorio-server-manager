package telemetry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func telemetryClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("telemetry redirects disabled") }}
}
func endpoint(base, path string) (string, error) {
	u, e := url.Parse(base)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid backend URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	return u.String(), nil
}
func send(ctx context.Context, client *http.Client, method, address, token string, body []byte, headers map[string]string) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if e != nil {
		return nil, errors.New("invalid backend request")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, errors.New("backend request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if e != nil || len(b) > 2<<20 {
		return nil, errors.New("invalid or oversized backend response")
	}
	return b, nil
}
