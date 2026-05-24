package hls

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Fetcher struct {
	client    *http.Client
	headers   map[string]string
	userAgent string
}

func NewFetcher(headers map[string]string, userAgent string, timeout time.Duration) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: timeout,
		},
		headers:   headers,
		userAgent: userAgent,
	}
}

func (f *Fetcher) Fetch(ctx context.Context, urlStr string) ([]byte, error) {
	delay := 1 * time.Second
	maxDelay := 60 * time.Second

	for {
		data, err := f.doFetch(urlStr)
		if err == nil {
			return data, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

func (f *Fetcher) FetchString(ctx context.Context, urlStr string) (string, error) {
	data, err := f.Fetch(ctx, urlStr)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (f *Fetcher) doFetch(urlStr string) ([]byte, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)
	for k, v := range f.headers {
		req.Header.Set(k, v)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", urlStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", urlStr, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", urlStr, err)
	}

	if cl := resp.ContentLength; cl > 0 {
		if int64(len(data)) != cl {
			return nil, fmt.Errorf("fetch %s: expected %d bytes, got %d", urlStr, cl, len(data))
		}
	}

	return data, nil
}
