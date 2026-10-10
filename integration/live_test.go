//go:build integration

// These opt-in tests use installed clients; normal CI uses HTTP/OS fixtures.
package integration_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wooto/lol-replay-recorder/lcu"
	"github.com/wooto/lol-replay-recorder/liveclient"
	"github.com/wooto/lol-replay-recorder/riotapi"
	"github.com/wooto/lol-replay-recorder/riotclient"
	"github.com/wooto/lol-replay-recorder/spectator"
)

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func TestLiveLCU(t *testing.T) {
	path := os.Getenv("LCU_LOCKFILE")
	if path == "" {
		t.Skip("set LCU_LOCKFILE for an already signed-in client")
	}
	client, err := lcu.FromLockfile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := liveContext(t)
	config, err := client.Configuration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if config.GameVersion == "" || !config.IsLoggedIn {
		t.Fatal("signed-in patched client required")
	}
	player, err := client.CurrentSummoner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	history, err := client.MatchHistory(ctx, player.PUUID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	region, err := client.Region(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LCU region=%s patch=%s history entries=%d", region.Region, config.GameVersion, len(history.Games.Games))
}

func TestLiveLCUReplayLaunch(t *testing.T) {
	if os.Getenv("RUN_LCU_LAUNCH_TESTS") != "1" {
		t.Skip("set RUN_LCU_LAUNCH_TESTS=1 to download/watch a recent replay and close its new game")
	}
	client, err := lcu.FromLockfile(os.Getenv("LCU_LOCKFILE"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	player, err := client.CurrentSummoner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	history, err := client.MatchHistory(ctx, player.PUUID, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	var replay lcu.Replay
	for _, match := range history.Games.Games {
		metadata, e := client.ReplayMetadata(ctx, match.GameID)
		if e != nil || metadata.State == "incompatible" {
			continue
		}
		replay, err = client.DownloadReplay(ctx, match.GameID, 100*time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if replay.GameID == 0 {
		t.Fatal("no available compatible recent replay")
	}
	process, err := client.LaunchReplay(ctx, replay, lcu.LaunchConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	api, err := spectator.New(spectator.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	for {
		playback, e := api.Playback(ctx)
		if e == nil && playback.Length > 0 {
			if _, err = api.Render(ctx); err != nil {
				t.Fatal(err)
			}
			t.Logf("LCU download/watch + spectator readback passed; replay length %.1fs", playback.Length)
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err = process.Close(); err != nil {
		t.Fatal(err)
	}
	if !process.Exited() {
		t.Fatal("owned game was not closed")
	}
}
func TestLiveRiotClient(t *testing.T) {
	line := os.Getenv("RIOT_CLIENT_COMMAND_LINE")
	if line == "" {
		t.Skip("provide running LeagueClientUx command line privately")
	}
	client, err := riotclient.FromCommandLine(line)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := liveContext(t)
	authorization, err := client.Authorization(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if authorization.Subject == "" {
		t.Fatal("signed-in Riot Client required")
	}
	sessions, err := client.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	region, err := client.Region(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Riot Client region=%s product sessions=%d", region.Region, len(sessions))
}
func TestLiveGameData(t *testing.T) {
	if os.Getenv("RUN_LIVE_GAME_TESTS") != "1" {
		t.Skip("set RUN_LIVE_GAME_TESTS=1 while a game is running")
	}
	client, err := liveclient.New(liveclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	data, err := client.AllGameData(liveContext(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("live game mode=%s time=%.1f players=%d", data.GameData.GameMode, data.GameData.GameTime, len(data.AllPlayers))
}
func TestLiveSpectator(t *testing.T) {
	if os.Getenv("RUN_LIVE_SPECTATOR_TESTS") != "1" {
		t.Skip("set RUN_LIVE_SPECTATOR_TESTS=1 with Replay API enabled")
	}
	client, err := spectator.New(spectator.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := liveContext(t)
	if _, err = client.Game(ctx); err != nil {
		t.Fatal(err)
	}
	playback, err := client.Playback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Render(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("spectator time=%.1f length=%.1f", playback.Time, playback.Length)
}
func TestLiveRiotAPI(t *testing.T) {
	key := os.Getenv("RIOT_API_KEY")
	target := os.Getenv("RIOT_TEST_TARGET")
	if key == "" || target == "" {
		t.Skip("set RIOT_API_KEY and RIOT_TEST_TARGET=gameName#tagLine")
	}
	name, tag, ok := strings.Cut(target, "#")
	if !ok {
		t.Fatal("full Riot ID required")
	}
	client, err := riotapi.New(riotapi.Config{APIKey: key, AccountBaseURL: os.Getenv("RIOT_REGIONAL_URL"), PlatformBaseURL: os.Getenv("RIOT_PLATFORM_URL")})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := liveContext(t)
	account, err := client.Lookup(ctx, name, tag)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := client.MatchIDs(ctx, account.PUUID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) > 0 {
		if _, err = client.Match(ctx, ids[0]); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("Riot API recent matches=%d", len(ids))
}
