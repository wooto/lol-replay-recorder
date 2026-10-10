package liveclient_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wooto/lol-replay-recorder/liveclient"
)

func TestLiveGameSnapshot(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/liveclientdata/allgamedata" || r.Header.Get("Authorization") != "" {
			w.WriteHeader(400)
			return
		}
		fmt.Fprint(w, `{"activePlayer":{"riotId":"선수#KR1","level":12},"allPlayers":[{"riotId":"선수#KR1","championName":"Ahri","team":"ORDER","isDead":false}],"events":{"Events":[{"EventID":1,"EventName":"ChampionKill","EventTime":135.5,"KillerName":"선수","VictimName":"상대"}]},"gameData":{"gameTime":140.2,"gameMode":"CLASSIC","mapNumber":11}}`)
	}))
	defer server.Close()
	client, err := liveclient.New(liveclient.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	snapshot, err := client.AllGameData(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.GameData.GameTime != 140.2 || len(snapshot.AllPlayers) != 1 || snapshot.Events.Events[0].EventName != "ChampionKill" {
		t.Fatal("live snapshot not decoded")
	}
}

func TestLiveSnapshotRejectsMissingGameData(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer server.Close()
	client, err := liveclient.New(liveclient.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.AllGameData(context.Background()); err == nil {
		t.Fatal("empty snapshot must not be reported as a live game")
	}
}

func TestPartialLiveDataAndIncrementalEvents(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/liveclientdata/playerlist":
			fmt.Fprint(w, `[{"riotId":"선수#KR1","championName":"Ahri","team":"ORDER"}]`)
		case "/liveclientdata/activeplayer":
			fmt.Fprint(w, `{"riotId":"선수#KR1","level":12}`)
		case "/liveclientdata/gamestats":
			fmt.Fprint(w, `{"gameTime":140.2,"mapNumber":11,"gameMode":"CLASSIC"}`)
		case "/liveclientdata/eventdata":
			if r.URL.Query().Get("eventID") != "4" {
				w.WriteHeader(400)
				return
			}
			fmt.Fprint(w, `{"Events":[{"EventID":4,"EventName":"DragonKill","EventTime":100}]}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := liveclient.New(liveclient.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	players, err := client.Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	active, err := client.ActivePlayer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	game, err := client.GameStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.Events(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if players[0].ChampionName != "Ahri" || active.Level != 12 || game.MapNumber != 11 || events.Events[0].EventID != 4 {
		t.Fatal("partial live data not decoded")
	}
}
