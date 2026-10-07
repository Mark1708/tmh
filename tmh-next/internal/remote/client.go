// Package remote implements the TUI-side control client for tmhd.
package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mark1708/tmh-next/internal/control"
	"github.com/mark1708/tmh-next/internal/domain"
)

type Client struct {
	http *http.Client
}

func New(socketPath string) (*Client, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("tmhd socket path is empty")
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		DisableCompression: true,
		MaxIdleConns:       4,
	}
	return &Client{http: &http.Client{Transport: transport}}, nil
}

func (c *Client) Snapshot(ctx context.Context) (control.Snapshot, error) {
	var snapshot control.Snapshot
	if err := c.request(ctx, http.MethodGet, "/v1/snapshot", nil, &snapshot); err != nil {
		return control.Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return control.Snapshot{}, fmt.Errorf("tmhd returned invalid snapshot: %w", err)
	}
	return snapshot, nil
}

func (c *Client) Search(ctx context.Context, query domain.SearchQuery) (domain.SearchResult, error) {
	var result domain.SearchResult
	if err := c.request(ctx, http.MethodPost, "/v1/search", query, &result); err != nil {
		return domain.SearchResult{}, err
	}
	return result, nil
}

func (c *Client) Execute(ctx context.Context, action domain.Action) (domain.MutationResult, error) {
	var result domain.MutationResult
	if err := c.request(ctx, http.MethodPost, "/v1/execute", action, &result); err != nil {
		return domain.MutationResult{}, err
	}
	return result, nil
}

func (c *Client) Watch(ctx context.Context, after uint64) (<-chan control.WatchEvent, <-chan error) {
	events := make(chan control.WatchEvent, 64)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		path := "/v1/watch?after=" + url.QueryEscape(strconv.FormatUint(after, 10))
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://tmhd"+path, nil)
		if err != nil {
			errs <- err
			return
		}
		response, err := c.http.Do(request)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				errs <- fmt.Errorf("watch tmhd: %w", err)
			}
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			errs <- decodeRemoteError(response)
			return
		}
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 64*1024), 1<<20)
		for scanner.Scan() {
			var event control.WatchEvent
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				errs <- fmt.Errorf("decode tmhd watch event: %w", err)
				return
			}
			if err := event.Validate(); err != nil {
				errs <- fmt.Errorf("invalid tmhd watch event: %w", err)
				return
			}
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			errs <- fmt.Errorf("read tmhd watch stream: %w", err)
		}
	}()
	return events, errs
}

func (c *Client) request(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode tmhd request: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://tmhd"+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("request tmhd: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeRemoteError(response)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<20))
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode tmhd response: %w", err)
	}
	return nil
}

func decodeRemoteError(response *http.Response) error {
	var payload struct {
		Code    domain.ErrCode `json:"code"`
		Message string         `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return fmt.Errorf("tmhd HTTP %d", response.StatusCode)
	}
	if payload.Code == domain.CodeRevisionConflict {
		return fmt.Errorf("%w: %s", domain.ErrRevisionConflict, payload.Message)
	}
	if payload.Code == "" || payload.Code == "unknown" {
		return fmt.Errorf("tmhd: %s", payload.Message)
	}
	return &domain.Error{Code: payload.Code, Msg: payload.Message}
}

var _ control.Client = (*Client)(nil)
var _ control.Watcher = (*Client)(nil)
