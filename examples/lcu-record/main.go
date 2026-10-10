// Command lcu-record downloads a known game via an already signed-in client
// and records through LCU's replay-watch launch and an owned game process.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	recorder "github.com/wooto/lol-replay-recorder"
	"github.com/wooto/lol-replay-recorder/lcu"
)

func main() {
	lockfile := flag.String("lockfile", `C:\Riot Games\League of Legends\lockfile`, "signed-in League client lockfile")
	gameID := flag.Uint64("game-id", 0, "numeric game ID already present in client replay metadata")
	target := flag.String("target", "", "full Riot ID in the replay")
	output := flag.String("output", "", "new WebM output path")
	probe := flag.String("ffprobe", "", "ffprobe executable")
	flag.Parse()
	if err := run(*lockfile, *gameID, *target, *output, *probe); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(lockfile string, gameID uint64, target, output, probe string) error {
	id, err := recorder.ParseRiotID(target)
	if err != nil {
		return err
	}
	if output == "" {
		return errors.New("output is required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	client, err := lcu.FromLockfile(lockfile)
	if err != nil {
		return err
	}
	defer client.Close()
	replay, err := client.DownloadReplay(ctx, gameID, 0)
	if err != nil {
		return err
	}
	r, err := recorder.New(recorder.Config{
		ProbeExecutable: probe,
		LaunchReplay: func(ctx context.Context, path string) (recorder.ReplayProcess, error) {
			if path != replay.Path {
				return nil, errors.New("recording replay path does not match downloaded game")
			}
			return client.LaunchReplay(ctx, replay, lcu.LaunchConfig{})
		},
		OnProgress: func(p recorder.Progress) {
			fmt.Fprintf(os.Stderr, "%s %.1f/%.1fs\n", p.Stage, p.CurrentSeconds, p.TotalSeconds)
		},
	})
	if err != nil {
		return err
	}
	result, err := r.RecordFull(ctx, recorder.Request{ReplayPath: replay.Path, Target: id, OutputPath: output})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
