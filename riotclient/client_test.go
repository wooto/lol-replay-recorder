package riotclient_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/wooto/lol-replay-recorder/riotclient"
)

func TestRiotClientUsesItsOwnRemotingCredentials(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, token, ok := r.BasicAuth()
		if !ok || user != "riot" || token != "riot-secret" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != "/rso-auth/v1/authorization" {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, `{"subject":"player-puuid","currentPlatformId":"KR","currentAccountId":123}`)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	command := `"C:\Riot Games\LeagueClientUx.exe" "--riotclient-app-port=` + u.Port() + `" "--riotclient-auth-token=riot-secret" --app-port=1 --remoting-auth-token=wrong-lcu-token`
	client, err := riotclient.FromCommandLine(command)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	auth, err := client.Authorization(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if auth.Subject != "player-puuid" || auth.CurrentPlatformID != "KR" {
		t.Fatalf("unexpected authorization: %+v", auth)
	}
}

func TestProductSessionsAndRegion(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/product-session/v1/sessions":
			fmt.Fprint(w, `{"league-session":{"productId":"league_of_legends","patchlineId":"live","launchConfiguration":{"executable":"C:/Riot Games/LeagueClient.exe"},"phase":"launched"}}`)
		case "/riotclient/region-locale":
			fmt.Fprint(w, `{"region":"KR","locale":"ko_KR"}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := riotclient.New(riotclient.Config{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	sessions, err := client.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	region, err := client.Region(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sessions["league-session"].ProductID != "league_of_legends" || region.Region != "KR" {
		t.Fatal("product session or region not decoded")
	}
}
