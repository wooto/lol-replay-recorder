# Client API integration

The client packages are independent. A Riot Web API key, an LCU remoting token,
a Riot Client remoting token and a spectator handoff key are different inputs.
The recorder still accepts a local replay without any of these integrations.

| Package | Public boundary | Capabilities |
| --- | --- | --- |
| `lcu` | `Client` | Current summoner, indexed match history, region, replay configuration/metadata, explicit metadata preparation, download/wait, watch, spectator launch |
| `riotclient` | `Client`, `LaunchProduct` | Signed-in authorization status, region, product sessions, Riot Client product/patchline CLI launch |
| `riotapi` | `Client` | Account-v1 Riot ID lookup, Match-v5 IDs/details, Spectator-v5 current game and context-aware waiting |
| `liveclient` | `Client` | Live game snapshot, players, active player, game stats and incremental events |
| `spectator` | `Client` | Replay game PID, playback, pause/seek/speed, camera state and partial camera updates |
| `observer` | Existing `Client.Capture` / replay handler | Experimental raw observer stream archival/serving; never a `.rofl` |
| `localclient` | Credential readers | League lockfile and explicit process-command-line credential parsing |

The regional host in `riotapi.Config.AccountBaseURL` routes both Account-v1 and
Match-v5 (Asia by default). `PlatformBaseURL` routes Spectator-v5 (KR by default).
The existing `discovery` import path remains compatible; `riotapi` exposes that
client together with its new match capabilities. HTTP 429 surfaces a
`StatusError.RetryAfter`; only `WaitForGame` retries automatically.

## Download and record through LCU

Use an already signed-in, idle League client with the Replay API enabled. A
numeric game ID must have usable replay metadata and the installed patch must
be compatible. `CreateReplayMetadata` can prepare known match metadata, but
does not grant replay access.

```powershell
go run ./examples/lcu-record -game-id 123456789 -target 'Player#KR1' -output C:\Videos\new-match.webm
```

The example downloads the replay, verifies its ready state and nonempty local
file, and connects `lcu.Client.LaunchReplay` to `recorder.Config.LaunchReplay`.
The caller's requested path must match the client-configured replay directory,
region and game ID. The Windows launcher rejects existing games, identifies a
single newly started `League of Legends.exe` and confirms its PID through
`/replay/game`. Its retained process handle closes that process only.
Do not start a separate game manually during this operation; simultaneous
manual launches cannot be reliably attributed to the watch request.
Errors never become a successful FULL recording.

`Watch` and `Spectate` are explicit launch requests and do not return process
ownership. `SpectateRequest` uses LCU's `puuid` and `spectatorKey` contract;
the code does not guess that the key is equivalent to Spectator-v5's
`observers.encryptionKey`. The latter remains an observer-stream handoff.

## Connections

```go
league, err := lcu.FromLockfile(lockfilePath)
if err != nil { return err }
// Close clients when done. Lockfiles/tokens can change when clients restart.
defer league.Close()

riot, err := riotclient.FromCommandLine(leagueClientUXCommandLine)
if err != nil { return err }
defer riot.Close()

live, err := liveclient.New(liveclient.Config{}) // https://127.0.0.1:2999
if err != nil { return err }
defer live.Close()

view, err := spectator.New(spectator.Config{})
if err != nil { return err }
defer view.Close()

web, err := riotapi.New(riotapi.Config{APIKey: apiKey})
if err != nil { return err }
defer web.Close()
```

The fragment shows the independent connection inputs. Read the LeagueClientUx command line privately
on Windows, or supply explicit `Config{BaseURL, Token}` credentials. Riot Client
uses `--riotclient-app-port`/`--riotclient-auth-token`; LCU uses
`--app-port`/`--remoting-auth-token`. No passwords are submitted or stored.

Local HTTP clients require numeric loopback origins, reject redirects, disable
proxies on their isolated standard transports, cap response bodies at 8 MiB,
reject null/trailing/malformed JSON, and preserve context cancellation.
`StrictTLS` requires a trusted certificate, including when the caller supplies
an `*http.Transport`. A custom nonstandard RoundTripper is a trusted test or
application boundary and must itself implement the intended transport policy.
Default certificate tolerance is isolated to the validated local origin.
Errors omit HTTP bodies and credentials. Never log tokens or process command
lines. `Credentials` formatting redacts its token.

Product launch uses the documented application convention
`--launch-product=league_of_legends --launch-patchline=live`, through an
explicitly supplied executable and `exec.CommandContext`, without a shell.
Call `Wait` on the returned command. Cancellation owns only the launcher
process, not any detached Riot/League product process.

## TDD and integration tests

Public client boundaries were agreed before test writing. Work progressed one
capability at a time: a failing public-interface test, implementation, then a
passing test. Tests use TLS HTTP servers as the external API boundary and a
real temporary Windows child process as the launcher/ownership boundary.
They never launch League in ordinary CI.

Covered flows include authenticated LCU configuration, credential source
separation, current-player history, replay metadata preparation, checking to
download to watch, an already-running download, ready-file verification,
busy-client rejection, LCU watch/spectate contracts, Riot product sessions/CLI
launch, Riot account to match/current-game, live snapshots/events and partial
Replay API playback/camera updates. Common integration tests cover
401/403/404/429/500, redirects, canceled requests, invalid origins/ports,
malformed/trailing/null/oversized responses and strict TLS.

```powershell
go test -count=1 ./...
go vet ./...
go build ./...
```

Live acceptance is explicit and separate:

```powershell
$env:LCU_LOCKFILE = 'C:\Riot Games\League of Legends\lockfile'
$env:RIOT_CLIENT_COMMAND_LINE = (Get-CimInstance Win32_Process -Filter "Name = 'LeagueClientUx.exe'").CommandLine
try {
    go test -tags integration -run 'TestLive(LCU|RiotClient)$' -v ./integration
} finally {
    Remove-Item Env:RIOT_CLIENT_COMMAND_LINE -ErrorAction SilentlyContinue
}
# Downloads/watches a compatible recent replay and closes only its new game:
$env:RUN_LCU_LAUNCH_TESTS = '1'
go test -tags integration -run TestLiveLCUReplayLaunch -v -timeout 3m ./integration
Remove-Item Env:RUN_LCU_LAUNCH_TESTS
```

`RUN_LIVE_GAME_TESTS=1` opts into Live Client Data reads during an active game.
`RUN_LIVE_SPECTATOR_TESTS=1` opts into existing spectator/replay reads.
The Riot Web API acceptance test requires `RIOT_API_KEY` and
`RIOT_TEST_TARGET=gameName#tagLine`; optional `RIOT_REGIONAL_URL` and
`RIOT_PLATFORM_URL` select other routes. Missing prerequisites cause a reported
skip, not a claim of live compatibility.

Observed on 2026-10-10 (KR, installed game version 16.20.824.8524):

- Actual LCU configuration/current summoner/history/region reads passed.
- Actual Riot Client authorization/region/product-session reads passed.
- A compatible recent replay download/watch, Replay API playback/render
  readback (877-second replay) and owned-process cleanup passed.
- Live Client Data on a live match was not exercised; no live match was running.
- Riot Web API production reads were not exercised; no developer key was supplied.

The existing recorder suite remains the FULL recording acceptance boundary.
The new live test verifies launch/readback/cleanup, not a new full-video result.

## Sources and contract provenance

- [Riot official League documentation](https://developer.riotgames.com/docs/lol):
  Live Client Data, Replay API, regional/platform routing and local API scope.
- [Riot official API catalog](https://developer.riotgames.com/apis):
  Account-v1, Match-v5 and Spectator-v5.
- [Riot League Director](https://github.com/RiotGames/LeagueDirector):
  official reference implementation of Replay API control.
- [League Akari replay client](https://github.com/LeagueAkari/LeagueAkari/blob/main/src/shared/http-api-axios-helper/league-client/replays.ts):
  primary application source for replay metadata/download/watch payloads.
- [League Akari spectator client](https://github.com/LeagueAkari/LeagueAkari/blob/main/src/shared/http-api-axios-helper/league-client/spectator.ts):
  primary application source for LCU spectator launch.
- [League Akari product launcher](https://github.com/LeagueAkari/LeagueAkari/blob/main/src/main/shards/client-installation/client-launcher.ts):
  primary application source for Riot Client launch flags.
- [Riot Client schema source](https://github.com/nomi-san/riot-client-schema):
  reference schema; actual installed-client swagger was also inspected.
- [1lann/lol-replay](https://github.com/1lann/lol-replay):
  historical primary implementation reference for the existing observer archive,
  not a guarantee of current-client playback.

LCU and Riot Client contracts are client-owned and version-dependent.
Installed-client help/swagger and live readbacks were used alongside references.
Unsupported responses remain errors; observer archive playback and cross-patch
recording are not inferred from a successful HTTP response.
