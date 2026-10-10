package lcu

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ReplayMetadata struct {
	GameID uint64 `json:"gameId"`
	State  string `json:"state"`
}
type Replay struct {
	GameID uint64
	Path   string
}
type MetadataRequest struct {
	GameVersion string `json:"gameVersion"`
	GameType    string `json:"gameType"`
	QueueID     int    `json:"queueId"`
	GameEnd     int64  `json:"gameEnd"`
}

// CreateReplayMetadata explicitly prepares client metadata for a known match.
// It does not grant replay access or prove that a download is available.
func (c *Client) CreateReplayMetadata(ctx context.Context, id uint64, request MetadataRequest) error {
	path, err := gamePath(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(request.GameVersion) == "" || strings.TrimSpace(request.GameType) == "" || request.QueueID < 0 || request.GameEnd < 0 {
		return errors.New("valid match metadata is required")
	}
	return c.http.Request(ctx, "POST", "/lol-replays/v2/metadata/"+path+"/create", request, nil)
}

type RegionLocale struct {
	Region string `json:"region"`
	Locale string `json:"locale"`
}

var ErrReplayUnavailable = errors.New("replay is unavailable")
var ErrReplayFile = errors.New("ready replay file is missing, empty or not regular")
var ErrClientBusy = errors.New("League client must be signed in, replay-enabled, idle and fully patched")

func (c *Client) idle(ctx context.Context) error {
	config, err := c.Configuration(ctx)
	if err != nil {
		return err
	}
	if !config.IsLoggedIn || !config.IsReplaysEnabled || config.IsPatching || config.IsPlayingGame || config.IsPlayingReplay {
		return ErrClientBusy
	}
	return nil
}

// Watch asks LCU to launch a ready replay. It does not claim process ownership;
// use Launcher when connecting to recorder.Config.LaunchReplay.
func (c *Client) Watch(ctx context.Context, id uint64) error {
	path, err := gamePath(id)
	if err != nil {
		return err
	}
	if err = c.idle(ctx); err != nil {
		return err
	}
	metadata, err := c.ReplayMetadata(ctx, id)
	if err != nil {
		return err
	}
	if metadata.State != "watch" {
		return ErrReplayUnavailable
	}
	return c.http.Request(ctx, "POST", "/lol-replays/v1/rofls/"+path+"/watch", map[string]string{"componentType": "replay-button_match-history"}, nil)
}

var platformPattern = regexp.MustCompile(`^[A-Z0-9]{2,8}$`)

func gamePath(id uint64) (string, error) {
	if id == 0 {
		return "", errors.New("game ID must be positive")
	}
	return strconv.FormatUint(id, 10), nil
}
func (c *Client) Region(ctx context.Context) (RegionLocale, error) {
	var region RegionLocale
	err := c.http.Request(ctx, "GET", "/riotclient/region-locale", nil, &region)
	if err == nil && !platformPattern.MatchString(region.Region) {
		err = errors.New("invalid client region")
	}
	return region, err
}
func (c *Client) ReplayMetadata(ctx context.Context, id uint64) (ReplayMetadata, error) {
	path, err := gamePath(id)
	if err != nil {
		return ReplayMetadata{}, err
	}
	var m ReplayMetadata
	err = c.http.Request(ctx, "GET", "/lol-replays/v1/metadata/"+path, nil, &m)
	if err == nil && (m.GameID != id || m.State == "") {
		err = errors.New("replay metadata identity or state missing")
	}
	return m, err
}

// DownloadReplay downloads through the logged-in client and waits for a ready,
// nonempty local file. A bounded default applies even without a caller deadline.
func (c *Client) DownloadReplay(ctx context.Context, id uint64, poll time.Duration) (Replay, error) {
	path, err := gamePath(id)
	if err != nil {
		return Replay{}, err
	}
	if ctx == nil {
		return Replay{}, errors.New("context is required")
	}
	if poll < 0 {
		return Replay{}, errors.New("poll interval cannot be negative")
	}
	if poll == 0 {
		poll = time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	m, err := c.ReplayMetadata(ctx, id)
	if err != nil {
		return Replay{}, err
	}
	if m.State == "incompatible" || m.State == "failed" || m.State == "lost" || m.State == "error" {
		return Replay{}, ErrReplayUnavailable
	}
	requested := false
	for {
		switch m.State {
		case "watch":
			region, err := c.Region(ctx)
			if err != nil {
				return Replay{}, err
			}
			var dir string
			if err = c.http.Request(ctx, "GET", "/lol-replays/v1/rofls/path", nil, &dir); err != nil {
				return Replay{}, err
			}
			if !filepath.IsAbs(dir) {
				return Replay{}, errors.New("client replay directory must be absolute")
			}
			file := filepath.Join(dir, replayPlatform(region.Region)+"-"+path+".rofl")
			info, err := os.Stat(file)
			if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				return Replay{}, ErrReplayFile
			}
			return Replay{GameID: id, Path: file}, nil
		case "found", "download":
			if !requested {
				if err = c.http.Request(ctx, "POST", "/lol-replays/v1/rofls/"+path+"/download", map[string]string{"componentType": "replay-button_match-history"}, nil); err != nil {
					return Replay{}, err
				}
				requested = true
			}
		case "checking", "downloading":
		default:
			return Replay{}, fmt.Errorf("%w: state %s", ErrReplayUnavailable, strings.TrimSpace(m.State))
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Replay{}, ctx.Err()
		case <-timer.C:
		}
		m, err = c.ReplayMetadata(ctx, id)
		if err != nil {
			return Replay{}, err
		}
	}
}
