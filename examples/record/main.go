// Command record demonstrates the library; it is not a scheduler or downloader.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"

	recorder "github.com/wooto/lol-replay-recorder"
)

func main() {
	replay := flag.String("replay", "", "local .rofl file")
	target := flag.String("target", "", "full Riot ID (gameName#tagLine)")
	output := flag.String("output", "", "new .webm output path")
	game := flag.String("game", "", "League game executable path")
	probe := flag.String("ffprobe", "", "ffprobe executable path")
	flag.Parse()
	if err := run(*replay, *target, *output, *game, *probe); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(replay, target, output, game, probe string) error {
	id, err := recorder.ParseRiotID(target)
	if err != nil {
		return err
	}
	client, err := recorder.New(recorder.Config{GameExecutable: game, ProbeExecutable: probe, OnProgress: func(progress recorder.Progress) {
		fmt.Fprintf(os.Stderr, "%s %.1f/%.1fs\n", progress.Stage, progress.CurrentSeconds, progress.TotalSeconds)
	}})
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	result, err := client.RecordFull(ctx, recorder.Request{ReplayPath: replay, Target: id, OutputPath: output})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
