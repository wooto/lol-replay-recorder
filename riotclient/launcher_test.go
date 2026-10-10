package riotclient_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wooto/lol-replay-recorder/riotclient"
)

// The child is the test executable, used as an OS process boundary fixture.
func init() {
	if file := os.Getenv("RIOT_LAUNCH_HELPER_OUTPUT"); file != "" {
		data, _ := json.Marshal(os.Args[1:])
		if os.WriteFile(file, data, 0600) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
}
func TestLaunchProductUsesDocumentedRiotClientArguments(t *testing.T) {
	file := filepath.Join(t.TempDir(), "arguments.json")
	t.Setenv("RIOT_LAUNCH_HELPER_OUTPUT", file)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command, err := riotclient.LaunchProduct(context.Background(), exe, "league_of_legends", "live")
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Wait(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err = json.Unmarshal(data, &args); err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "--launch-product=league_of_legends" || args[1] != "--launch-patchline=live" {
		t.Fatalf("unexpected launch arguments: %v", args)
	}
}
