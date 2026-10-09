# lol-replay-recorder

A Go library for recording a local League of Legends replay with the camera
smoothly following one player, with experimental observer-stream archiving and replay
HTTP serving. Video recording requires Windows amd64. MIT licensed; Go 1.26+.

This branch replaces the historical TypeScript/npm implementation. Existing Git
history and npm tags remain intact. **v0.1.0-alpha.2** passed one current-patch KR
local-replay FULL recording on Windows: windowed 1280×720, requested 30 FPS,
498.412 seconds of replay and 498.304 seconds of WebM. Video packet bounds,
full video decoding, target/death/respawn checks, and owned-process cleanup passed.
This is an experimental release; other patches, maps, profiles, and observer
archive playback remain unverified. See [live validation](docs/live-recording-validation.md).

```powershell
go get github.com/wooto/lol-replay-recorder@v0.1.0-alpha.2
```

| Package | Responsibility |
| --- | --- |
| `recorder` (module root) | Launch a compatible local `.rofl`, follow one player, and capture WebM on Windows amd64 |
| `observer` | Save opaque observer chunks and keyframes into a custom archive; expose a completed archive through an HTTP handler |
| `discovery` | Optional Riot Account-v1 / Spectator-v5 lookup and context-aware waiting for an active game; caller supplies the API key |

Observer archives are **not `.rofl` files** and cannot be passed to
`recorder.RecordFull`. The replay handler does not start or authenticate a League
client. Archive-to-game launch and current-client playback are unverified. The
observer and discovery packages use portable Go; the video recorder remains
Windows-only. Nothing depends on OP.GG.

## Requirements

- Go 1.26 or newer; Windows amd64 with an unlocked interactive desktop.
- An installed League client and a nonempty `.rofl` compatible with its patch.
- Replay API enabled in the game's `Config/game.cfg`:
  ```ini
  [General]
  EnableReplayApi=1
  ```
- `ffprobe` on PATH, or `Config.ProbeExecutable` set to its executable path.
- For keyboard fallback, standard spectator player bindings `1–5` and `Q–T`, or explicit
  `Config.SelectionKeys`. The library does not rewrite game settings.
- No existing game using the local Replay API. The library takes game-window focus.

## Use

Import path: `github.com/wooto/lol-replay-recorder` (package `recorder`). Pin the
alpha version explicitly; it is not a stable release.

```go
target, err := recorder.ParseRiotID("Player#KR1")
if err != nil { return err }
r, err := recorder.New(recorder.Config{
    GameExecutable: `C:\Riot Games\League of Legends\Game\League of Legends.exe`,
    ProbeExecutable: `C:\Tools\ffprobe.exe`,
})
if err != nil { return err }
result, err := r.RecordFull(ctx, recorder.Request{
    ReplayPath: `C:\Replays\match.rofl`,
    Target: target,
    OutputPath: `C:\Videos\match.webm`,
})
if err != nil { return err }
fmt.Println(result.Path)
```

The output directory must exist and the output file must not exist. Defaults are
1920×1080 at 60 FPS, normal playback speed, and WebM. The library launches the
replay, pauses and prepares at 0.1 seconds (champion objects are absent at exact
zero on the tested client), identifies the complete Riot ID, selects through
Replay API with verified keyboard fallback, and verifies camera attachment.
A constant selection-name sequence reapplies the target through encoder seeks
and respawns. An elevated camera height and 56-degree angle are also maintained
and verified: attachment alone can otherwise leave the camera inside terrain.
Horizontal following is eased instead of rigidly centering the player: the
recorder reads camera position and attachment offset through Replay API and
updates the offset as the selected player moves. It retains that player rather
than switching to other fights. This is library-controlled camera motion, not
the client's native directed spectator camera or the player's original inputs.
On the tested patch, FPS capture skips the first five seconds with `startTime=0`.
The recorder requests a five-second native pre-roll (`startTime=-5`) so the video
can begin at game time zero. This is an observed client workaround, not a Riot
compatibility guarantee. It monitors
the camera and recording state, allows an empty selection during explicitly
confirmed target death with the camera still attached while retaining the target
track, requires observed start and completion covering
the replay length, then checks file stability, decodes video frames with ffprobe,
and verifies the first and final video packet timestamps. Audio/container length
alone cannot prove full video coverage. Packet metadata is parsed as a stream.
Playback starts before the encoder, following League Director's recording order.
Capture uses real-time mode (`enforceFrameRate=false`): the current client's
accelerated mode produced shortened output during local tests. Completion also
handles the native `endTime=-1` sentinel without skipping target-camera or media
validation. Empty attached selections are retried once; the death exception
requires an explicit target death with a clock aligned to the recording.

Errors return no successful result. `*recorder.Error` includes the stage and any
partial output path; partial video is retained. Use `errors.Is` for cancellation
or sentinel errors. Only the process launched by this call is closed. At most one
recording can use the local client per Windows session. Progress callbacks are
synchronous and should return promptly.

Replay HTTP traffic stays on numeric loopback, with proxies and redirects
disabled. The default transport tolerates the game's local certificate; set
`StrictTLS` to require OS certificate trust. This does not change global TLS.

The default launch uses the `.rofl` as its first argument and the installation's
`-GameBaseDir`. `ExtraLaunchArgs` can override these additional arguments. Current
direct executable launch remains unverified on the tested installation;
unsupported or unverifiable behavior fails instead of reporting FULL success.

Current Windows installations can deny direct game executable launch even while
the logged-in League client's replay-watch operation succeeds. Applications can
set `Config.LaunchReplay` to launch through their client integration and return
an owned `recorder.ReplayProcess` (`PID`, `Exited`, `Close`). The recorder still
checks API/process identity and closes only that owned game. The callback must
not adopt an already running game and must clean up its own failed launches.
This hook adds no login, replay-download, or LCU dependency to the recording core.

Local `.rofl` downloading, Riot login, player-directory crawling, uploading, and
transcoding belong to the calling application. Optional active-game lookup is in
the separate `discovery` package.

## Experimental observer archive

The observed HTTP protocol is separate from LCU client control and the official
Riot Web API. It has no third-party compatibility guarantee. A successful server
`version` response is only a reachability check, not proof that game chunks or
offline replay will work.

```go
client, err := observer.NewClient(observer.ClientConfig{
    BaseURL: "http://spectator.kr.lol.pvp.net:8080",
})
if err != nil { return err }
archive, err := client.Capture(ctx, observer.Game{
    PlatformID: "KR",
    GameID: gameID, // caller obtains a currently observable game's numeric ID
}, `C:\Archives\new-match`)
if err != nil { return err } // any partial archive is retained, never marked complete
handler, err := observer.NewReplayHandler(archive)
if err != nil { return err }
// Attach handler to a caller-owned HTTP server bound to numeric loopback.
_ = handler
```

`Capture` requires a new output directory, preserves chunks as opaque bytes,
saves metadata and the observed server version, and waits for the announced end
of the stream. Completion requires the expected contiguous chunk and keyframe
set. A complete archive means **complete relative to the observed protocol**;
it does not establish match-time-zero coverage, camera POV, current-client
playability, or a complete video. Unsupported responses fail rather than
inventing missing data. Partial archives can be inspected but are rejected by
the replay handler. Archive readers reject malformed manifests and unsafe paths.

Examples from a checkout:

```powershell
go run ./examples/observe -game-id 123456789 -platform KR -output C:\Archives\new-match
go run ./examples/serve -archive C:\Archives\new-match
```

To wait for a selected KR account instead of supplying a known game ID, set your
own Riot developer API key in `RIOT_API_KEY`, then run:

```powershell
go run ./examples/observe -target 'Player#KR1' -output C:\Archives\new-match
```

This uses Account-v1 in Asia and Spectator-v5 on KR. The example polls every ten
seconds and does **not** guarantee detection before the first chunk. API
availability, privacy settings, authorization, server retention, and protocol
changes may prevent collection. Cancellation interrupts polling and requests;
rate-limit waits respect `Retry-After`. No password login is automated.

The library does not claim that this internal observer protocol is an approved
Riot developer API or a way around Private Replays. Riot's announced replay
restriction was [delayed to 26.21](https://x.com/LeagueOfLegends/status/2107875798825021469);
this is not a promise of future observer access.

## Development and live acceptance

```powershell
go mod verify
go vet ./...
go test -count=1 ./...
go build ./...
go run ./examples/record -replay C:\Replays\match.rofl -target 'Player#KR1' -output C:\Videos\match.webm
```

The regular test suite never launches League. To explicitly run real-game
acceptance on a prepared Windows PC:

```powershell
$env:LOL_REPLAY_PATH = 'C:\Replays\match.rofl'
$env:LOL_REPLAY_TARGET = 'Player#KR1'
$env:LOL_GAME_EXECUTABLE = 'C:\Riot Games\League of Legends\Game\League of Legends.exe'
$env:LOL_FFPROBE = 'C:\Tools\ffprobe.exe'
go test -tags e2e -run TestLiveFullRecording -count=1 -timeout 3h
```

Before publishing a stable Go version, verify a current-patch full replay, both team camera
bindings, Unicode paths, cancellation, and existing-output preservation. Inspect
the beginning and ending of the resulting video and confirm the selected player
stays attached. The E2E test's temporary video is removed after the test; use the
example command when retaining an acceptance video.

CI checks formatting, modules, vet, tests and builds on Windows using minimum and
stable Go versions. Linux runs race checks for orchestration and vulnerability
checks for both platforms; this does not imply Linux recording support. New Go
tags start at `v0.1.0-alpha.1`; the release workflow validates main-branch
ancestry, formatting, tests, builds, and vulnerability checks. Numbered
alpha/beta/rc tags publish GitHub prereleases. Stable tags create drafts for
manual live acceptance. Any pushed Go tag is fetchable even if its GitHub
release is a draft. Alpha tags intentionally distribute the experimental API
without claiming live acceptance.

Protocol reference: [Riot Replay API documentation](https://developer.riotgames.com/docs/lol#game-client-api_replay-api).
Implementation references and known limitations: [observer design notes](docs/observer-design.md).
