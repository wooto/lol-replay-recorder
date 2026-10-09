package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAccountBaseURL  = "https://asia.api.riotgames.com"
	defaultPlatformBaseURL = "https://kr.api.riotgames.com"
	defaultRequestTimeout  = 10 * time.Second
	maxResponseBytes       = 1 << 20
)

var (
	// ErrNoActiveGame is returned only when the Spectator API reports HTTP 404.
	ErrNoActiveGame = errors.New("player has no active game")
	// ErrAccountNotFound identifies a Riot ID lookup that returned HTTP 404.
	ErrAccountNotFound = errors.New("Riot account not found")
	// ErrRedirect is returned when an API request attempts to follow a redirect.
	ErrRedirect = errors.New("Riot API redirects are not allowed")
	// ErrInvalidResponse indicates a successful API response was malformed.
	ErrInvalidResponse = errors.New("invalid Riot API response")
	// ErrRequestFailed indicates the client could not complete an API request.
	ErrRequestFailed = errors.New("Riot API request failed")
)

// Config configures the optional Riot API client. APIKey is sent only in the
// X-Riot-Token header. The base URLs default to Riot's official regional
// Account-v1 and platform-specific Spectator-v5 hosts.
//
// HTTPClient may supply a custom transport. The client is copied, its redirect
// policy is replaced to reject redirects, and a request timeout is added when
// the supplied client has none.
// Custom base URLs must use HTTPS, except loopback HTTP URLs used by local test
// servers.
type Config struct {
	APIKey          string
	HTTPClient      *http.Client
	AccountBaseURL  string
	PlatformBaseURL string
}

// Client calls Riot's Account-v1 and Spectator-v5 APIs.
type Client struct {
	apiKey          string
	httpClient      *http.Client
	accountBaseURL  *url.URL
	platformBaseURL *url.URL
}

// Account contains the PUUID returned by Riot's Account-v1 API.
type Account struct {
	PUUID string
}

// Game contains the active-game information required to connect a local
// observer. EncryptionKey is returned in memory for the caller's local game
// handoff; this package never persists it.
type Game struct {
	GameID        uint64
	PlatformID    string
	GameStartTime int64
	EncryptionKey string
}

// StatusError describes an HTTP status returned by Riot. RetryAfter contains
// the parsed Retry-After value (zero when the header is absent or invalid).
// It never includes response bodies or request credentials.
type StatusError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("Riot API returned HTTP %d", e.StatusCode)
}

type accountNotFoundError struct{ status *StatusError }

func (e *accountNotFoundError) Error() string {
	return ErrAccountNotFound.Error() + ": " + e.status.Error()
}
func (e *accountNotFoundError) Unwrap() error { return e.status }
func (e *accountNotFoundError) Is(target error) bool {
	return target == ErrAccountNotFound
}

// New validates the configuration and constructs a client. No network request
// is made until Lookup, ActiveGame, or WaitForGame is called.
func New(config Config) (*Client, error) {
	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		return nil, errors.New("APIKey is required")
	}
	accountBase, err := parseBaseURL(config.AccountBaseURL, defaultAccountBaseURL)
	if err != nil {
		return nil, fmt.Errorf("AccountBaseURL: %w", err)
	}
	platformBase, err := parseBaseURL(config.PlatformBaseURL, defaultPlatformBaseURL)
	if err != nil {
		return nil, fmt.Errorf("PlatformBaseURL: %w", err)
	}

	httpClient := &http.Client{}
	if config.HTTPClient != nil {
		copy := *config.HTTPClient
		*httpClient = copy
	}
	if httpClient.Timeout <= 0 {
		httpClient.Timeout = defaultRequestTimeout
	}
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return ErrRedirect
	}

	return &Client{
		apiKey:          apiKey,
		httpClient:      httpClient,
		accountBaseURL:  accountBase,
		platformBaseURL: platformBase,
	}, nil
}

func parseBaseURL(raw, fallback string) (*url.URL, error) {
	custom := raw != ""
	if !custom {
		raw = fallback
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Opaque != "" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("must be an absolute URL without credentials, query, or fragment")
	}
	if u.Scheme != "https" {
		if !(custom && u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
			return nil, errors.New("must use HTTPS (HTTP is allowed only for loopback test servers)")
		}
	}
	if strings.Contains(raw, "#") {
		return nil, errors.New("must not contain a fragment")
	}
	if u.RawPath != "" {
		// url.URL.String handles valid RawPath values. Reject invalid ones instead
		// of silently changing the configured endpoint.
		if _, err := url.PathUnescape(u.RawPath); err != nil {
			return nil, errors.New("contains an invalid escaped path")
		}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Lookup resolves a Riot ID through Account-v1 and returns its PUUID.
func (c *Client) Lookup(ctx context.Context, gameName, tagLine string) (Account, error) {
	if err := checkContextAndID(ctx, gameName, "gameName"); err != nil {
		return Account{}, err
	}
	if err := checkContextAndID(ctx, tagLine, "tagLine"); err != nil {
		return Account{}, err
	}
	route := "/riot/account/v1/accounts/by-riot-id/" + url.PathEscape(gameName) + "/" + url.PathEscape(tagLine)
	var response struct {
		PUUID string `json:"puuid"`
	}
	err := c.get(ctx, c.accountBaseURL, route, &response)
	if err != nil {
		var status *StatusError
		if errors.As(err, &status) && status.StatusCode == http.StatusNotFound {
			return Account{}, &accountNotFoundError{status: status}
		}
		return Account{}, err
	}
	if strings.TrimSpace(response.PUUID) == "" {
		return Account{}, ErrInvalidResponse
	}
	return Account{PUUID: response.PUUID}, nil
}

// ActiveGame fetches the player's active-game observer handoff from
// Spectator-v5. HTTP 404 maps to ErrNoActiveGame.
func (c *Client) ActiveGame(ctx context.Context, puuid string) (Game, error) {
	if err := checkContextAndID(ctx, puuid, "puuid"); err != nil {
		return Game{}, err
	}
	route := "/lol/spectator/v5/active-games/by-summoner/" + url.PathEscape(puuid)
	var response struct {
		GameID        uint64 `json:"gameId"`
		PlatformID    string `json:"platformId"`
		GameStartTime int64  `json:"gameStartTime"`
		Observers     struct {
			EncryptionKey string `json:"encryptionKey"`
		} `json:"observers"`
	}
	if err := c.get(ctx, c.platformBaseURL, route, &response); err != nil {
		var status *StatusError
		if errors.As(err, &status) && status.StatusCode == http.StatusNotFound {
			return Game{}, ErrNoActiveGame
		}
		return Game{}, err
	}
	if response.GameID == 0 || strings.TrimSpace(response.PlatformID) == "" || strings.TrimSpace(response.Observers.EncryptionKey) == "" {
		return Game{}, ErrInvalidResponse
	}
	return Game{
		GameID:        response.GameID,
		PlatformID:    response.PlatformID,
		GameStartTime: response.GameStartTime,
		EncryptionKey: response.Observers.EncryptionKey,
	}, nil
}

// WaitForGame polls Spectator-v5 until a game is found or ctx is canceled. A
// 429 response uses Riot's Retry-After delay when present; other statuses,
// including authentication and authorization failures, return immediately.
func (c *Client) WaitForGame(ctx context.Context, puuid string, pollInterval time.Duration) (Game, error) {
	if pollInterval <= 0 {
		return Game{}, errors.New("poll interval must be positive")
	}
	if err := checkContextAndID(ctx, puuid, "puuid"); err != nil {
		return Game{}, err
	}
	for {
		game, err := c.ActiveGame(ctx, puuid)
		if err == nil {
			return game, nil
		}
		if errors.Is(err, ErrNoActiveGame) {
			if err := waitContext(ctx, pollInterval); err != nil {
				return Game{}, err
			}
			continue
		}
		var status *StatusError
		if errors.As(err, &status) && status.StatusCode == http.StatusTooManyRequests {
			delay := status.RetryAfter
			if delay <= 0 {
				delay = pollInterval
			}
			if err := waitContext(ctx, delay); err != nil {
				return Game{}, err
			}
			continue
		}
		return Game{}, err
	}
}

func checkContextAndID(ctx context.Context, value, name string) error {
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	return nil
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) get(ctx context.Context, base *url.URL, route string, out any) error {
	requestURL := base.String() + route
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return ErrInvalidResponse
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Riot-Token", c.apiKey)

	response, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if errors.Is(err, ErrRedirect) {
			return ErrRedirect
		}
		// Hide transport error text so a custom transport cannot accidentally
		// include request headers or credentials in a user-facing error.
		return ErrRequestFailed
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return &StatusError{
			StatusCode: response.StatusCode,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now()),
		}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(out); err != nil {
		return ErrInvalidResponse
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrInvalidResponse
	}
	return nil
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		maxSeconds := int64(math.MaxInt64) / int64(time.Second)
		if seconds > maxSeconds {
			return time.Duration(math.MaxInt64)
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}
