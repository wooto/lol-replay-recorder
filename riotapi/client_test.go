package riotapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wooto/lol-replay-recorder/riotapi"
)

func TestRiotAccountToMatchAndActiveGame(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Riot-Token") != "developer-key" || r.URL.Query().Has("api_key") {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/riot/account/v1/accounts/by-riot-id/선수/KR1":
			fmt.Fprint(w, `{"puuid":"player-id"}`)
		case "/lol/match/v5/matches/by-puuid/player-id/ids":
			if r.URL.Query().Get("start") != "0" || r.URL.Query().Get("count") != "2" {
				w.WriteHeader(400)
				return
			}
			fmt.Fprint(w, `["KR_123"]`)
		case "/lol/match/v5/matches/KR_123":
			fmt.Fprint(w, `{"metadata":{"matchId":"KR_123","participants":["player-id"]},"info":{"gameId":123,"gameVersion":"26.20.1","queueId":420,"participants":[{"puuid":"player-id","riotIdGameName":"선수","riotIdTagline":"KR1","championName":"Ahri","kills":10,"deaths":2,"assists":8}]}}`)
		case "/lol/spectator/v5/active-games/by-summoner/player-id":
			fmt.Fprint(w, `{"gameId":124,"platformId":"KR","gameStartTime":1,"observers":{"encryptionKey":"observer-key"}}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := riotapi.New(riotapi.Config{APIKey: "developer-key", AccountBaseURL: server.URL, PlatformBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	account, err := client.Lookup(context.Background(), "선수", "KR1")
	if err != nil {
		t.Fatal(err)
	}
	ids, err := client.MatchIDs(context.Background(), account.PUUID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	match, err := client.Match(context.Background(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	game, err := client.ActiveGame(context.Background(), account.PUUID)
	if err != nil {
		t.Fatal(err)
	}
	if match.Info.Participants[0].Kills != 10 || match.Metadata.MatchID != "KR_123" || game.GameID != 124 {
		t.Fatal("Riot flow not decoded")
	}
}
