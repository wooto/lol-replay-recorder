// Command observe saves an experimental observer stream, not a video or ROFL.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	recorder "github.com/wooto/lol-replay-recorder"
	"github.com/wooto/lol-replay-recorder/discovery"
	"github.com/wooto/lol-replay-recorder/observer"
)

func main() {
	target := flag.String("target", "", "full Riot ID to wait for on KR (requires RIOT_API_KEY)")
	gameID := flag.Uint64("game-id", 0, "known active game ID; alternative to target")
	platform := flag.String("platform", "KR", "platform for a known game ID")
	source := flag.String("source", "http://spectator.kr.lol.pvp.net:8080", "observer HTTP(S) origin")
	output := flag.String("output", "", "new archive directory")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, *target, *gameID, *platform, *source, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, target string, gameID uint64, platform, source, output string) error {
	if output == "" || (target == "") == (gameID == 0) {
		return errors.New("provide output and exactly one of target or game-id")
	}
	game := observer.Game{PlatformID: platform, GameID: gameID}
	if target != "" {
		if platform != "KR" {
			return errors.New("the target lookup example supports KR; use game-id for other platforms")
		}
		id, err := recorder.ParseRiotID(target)
		if err != nil {
			return err
		}
		client, err := discovery.New(discovery.Config{APIKey: os.Getenv("RIOT_API_KEY")})
		if err != nil {
			return err
		}
		account, err := client.Lookup(ctx, id.GameName, id.TagLine)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Waiting for an observable active game; detection can miss the beginning.")
		active, err := client.WaitForGame(ctx, account.PUUID, 10*time.Second)
		if err != nil {
			return err
		}
		game = observer.Game{PlatformID: active.PlatformID, GameID: active.GameID}
	}
	client, err := observer.NewClient(observer.ClientConfig{BaseURL: source})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Capturing %s/%d into %s\n", game.PlatformID, game.GameID, output)
	archive, err := client.Capture(ctx, game, output)
	if err != nil {
		return fmt.Errorf("capture failed; any partial archive remains in %s: %w", output, err)
	}
	return json.NewEncoder(os.Stdout).Encode(archive.Manifest())
}
