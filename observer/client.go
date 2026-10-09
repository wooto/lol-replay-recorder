package observer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultObserverResponseBytes = int64(64 << 20)
	defaultObserverHTTPTimeout   = 10 * time.Second
	minObserverPollInterval      = 100 * time.Millisecond
	maxObserverPollInterval      = time.Minute
)

// Client downloads raw data from a League spectator observer consumer server.
// It does not convert the downloaded data into a RoFL replay.
type Client struct {
	baseURL          *url.URL
	httpClient       *http.Client
	pollInterval     time.Duration
	maxResponseBytes int64
}

type observerHTTPError struct {
	status int
	path   string
}

func (e *observerHTTPError) Error() string {
	return fmt.Sprintf("observer request %s returned HTTP %d", e.path, e.status)
}

func isNotFound(err error) bool {
	var httpErr *observerHTTPError
	return errors.As(err, &httpErr) && httpErr.status == http.StatusNotFound
}

// NewClient validates the explicit observer server URL and applies bounded
// defaults. Redirects are disabled even when the caller supplies an HTTP client.
func NewClient(config ClientConfig) (*Client, error) {
	base, err := parseObserverBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	if config.PollInterval < 0 {
		return nil, errors.New("poll interval cannot be negative")
	}
	pollInterval := config.PollInterval
	if pollInterval == 0 {
		pollInterval = time.Second
	} else if pollInterval < minObserverPollInterval {
		pollInterval = minObserverPollInterval
	} else if pollInterval > maxObserverPollInterval {
		pollInterval = maxObserverPollInterval
	}
	maxBytes := config.MaxResponseBytes
	if maxBytes == 0 {
		maxBytes = defaultObserverResponseBytes
	}
	if maxBytes < 1 || maxBytes > maxArchiveFileBytes {
		return nil, fmt.Errorf("max response bytes must be between 1 and %d", maxArchiveFileBytes)
	}

	client := &http.Client{}
	if config.HTTPClient != nil {
		copy := *config.HTTPClient
		client = &copy
	}
	if client.Timeout <= 0 {
		client.Timeout = defaultObserverHTTPTimeout
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{
		baseURL:          base,
		httpClient:       client,
		pollInterval:     pollInterval,
		maxResponseBytes: maxBytes,
	}, nil
}

func parseObserverBaseURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("observer BaseURL must be explicitly set")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid observer BaseURL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" || u.Opaque != "" {
		return nil, errors.New("observer BaseURL must be an absolute HTTP or HTTPS URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" {
		return nil, errors.New("observer BaseURL cannot contain credentials, query, or fragment")
	}
	if strings.ContainsAny(u.Path, "\\\x00\r\n") {
		return nil, errors.New("observer BaseURL has an invalid path")
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, errors.New("observer BaseURL cannot contain dot path segments")
		}
	}
	return u, nil
}

func (c *Client) endpoint(path string) string {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawPath = ""
	return u.String()
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	return c.getBounded(ctx, path, c.maxResponseBytes)
}

func (c *Client) getBounded(ctx context.Context, path string, resourceLimit int64) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("context cannot be nil")
	}
	limit := minResponseLimit(c.maxResponseBytes, resourceLimit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, application/octet-stream, text/plain")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &observerHTTPError{status: response.StatusCode, path: path}
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("observer response %s exceeds %d bytes", path, limit)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("observer response %s exceeds %d bytes", path, limit)
	}
	return data, nil
}

func (c *Client) fetchVersion(ctx context.Context) (string, error) {
	data, err := c.getBounded(ctx, "/observer-mode/rest/consumer/version", maxVersionBytes)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(data))
	if len(version) > 0 && version[0] == '"' {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return "", fmt.Errorf("decode observer version: %w", err)
		}
		version = strings.TrimSpace(decoded)
	}
	if version == "" || strings.ContainsAny(version, "\r\n\x00") {
		return "", errors.New("observer returned an invalid version")
	}
	return version, nil
}

func (c *Client) fetchMetadata(ctx context.Context, game Game) ([]byte, observerMetadata, error) {
	path := fmt.Sprintf("/observer-mode/rest/consumer/getGameMetaData/%s/%d/0/token", game.PlatformID, game.GameID)
	data, err := c.getBounded(ctx, path, maxMetadataBytes)
	if err != nil {
		return nil, observerMetadata{}, err
	}
	metadata, err := parseMetadata(data, game, true)
	if err != nil {
		return nil, observerMetadata{}, fmt.Errorf("decode observer metadata: %w", err)
	}
	return data, metadata, nil
}

func (c *Client) fetchLastChunkInfo(ctx context.Context, game Game) ([]byte, ChunkInfo, error) {
	path := fmt.Sprintf("/observer-mode/rest/consumer/getLastChunkInfo/%s/%d/0/token", game.PlatformID, game.GameID)
	data, err := c.getBounded(ctx, path, maxChunkInfoBytes)
	if err != nil {
		return nil, ChunkInfo{}, err
	}
	info, err := parseChunkInfo(data)
	if err != nil {
		return nil, ChunkInfo{}, fmt.Errorf("decode observer last chunk info: %w", err)
	}
	return data, info, nil
}

func parseChunkInfo(data []byte) (ChunkInfo, error) {
	var fields map[string]json.RawMessage
	if err := decodeSingleJSON(data, &fields, false); err != nil {
		return ChunkInfo{}, err
	}
	if fields == nil {
		return ChunkInfo{}, errors.New("last chunk info must be a JSON object")
	}
	if raw, ok := fields["chunkId"]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ChunkInfo{}, errors.New("last chunk info is missing a valid chunkId")
	}
	for _, key := range []string{"chunkId", "keyFrameId", "nextChunkId", "endStartupChunkId", "startGameChunkId", "endGameChunkId", "duration", "availableSince", "nextAvailableChunk"} {
		if raw, ok := fields[key]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return ChunkInfo{}, fmt.Errorf("last chunk info field %s cannot be null", key)
		}
	}
	var info ChunkInfo
	if err := decodeSingleJSON(data, &info, false); err != nil {
		return ChunkInfo{}, err
	}
	if err := validateChunkInfo(info); err != nil {
		return ChunkInfo{}, err
	}
	return info, nil
}

func (c *Client) fetchPayload(ctx context.Context, game Game, kind string, id uint64) ([]byte, error) {
	var endpoint string
	switch kind {
	case "chunks":
		endpoint = "getGameDataChunk"
	case "keyframes":
		endpoint = "getKeyFrame"
	default:
		return nil, errors.New("unknown observer payload type")
	}
	path := fmt.Sprintf("/observer-mode/rest/consumer/%s/%s/%d/%d/token", endpoint, game.PlatformID, game.GameID, id)
	return c.get(ctx, path)
}

func (c *Client) fetchEndStats(ctx context.Context, game Game) ([]byte, error) {
	path := fmt.Sprintf("/observer-mode/rest/consumer/endOfGameStats/%s/%d/null", game.PlatformID, game.GameID)
	data, err := c.getBounded(ctx, path, maxEndStatsBytes)
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, errors.New("observer returned invalid end-of-game statistics JSON")
	}
	return data, nil
}

func minResponseLimit(configured, resource int64) int64 {
	if configured < resource {
		return configured
	}
	return resource
}
