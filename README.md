# lol-replay-recorder

A Go library for recording an entire local League of Legends replay with the
camera attached to one player. Windows amd64 only. MIT licensed.

This branch replaces the historical TypeScript/npm implementation. Existing Git
history and npm tags remain intact. No Go release has been published yet:
**current-patch, real-game acceptance is pending**. Automated tests simulate the
Replay API; they do not establish that a current client can launch and capture.

## Requirements

- Go 1.26 or newer; Windows amd64 with an unlocked interactive desktop.
- An installed League client and a nonempty `.rofl` compatible with its patch.
- Replay API enabled in the game's `Config/game.cfg`:
  ```ini
  [General]
  EnableReplayApi=1
  ```
- `ffprobe` on PATH, or `Config.ProbeExecutable` set to its executable path.
- Standard spectator player bindings `1–5` and `Q–T`, or explicit
  `Config.SelectionKeys`. The library does not rewrite game settings.
- No existing game using the local Replay API. The library takes game-window focus.

## Use

Import path: `github.com/wooto/lol-replay-recorder` (package `recorder`). Until a Go
release is validated, use a local checkout or an explicitly reviewed commit.

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
replay, pauses and seeks to zero, identifies the complete Riot ID, double-selects
the player, and verifies camera attachment before starting capture. It monitors
the camera and recording state, requires observed start and completion covering
the replay length, then checks file stability and decodes frames with ffprobe.

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
client launch, selection-name fields, and encoding behavior need live validation;
unsupported or unverifiable behavior fails instead of reporting FULL success.

Player discovery, account lookup, replay downloading, authentication, uploading,
and transcoding belong to the calling application.

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

Before tagging a Go version, verify a current-patch full replay, both team camera
bindings, Unicode paths, cancellation, and existing-output preservation. Inspect
the beginning and ending of the resulting video and confirm the selected player
stays attached. The E2E test's temporary video is removed after the test; use the
example command when retaining an acceptance video.

CI checks formatting, modules, vet, tests and builds on Windows using minimum and
stable Go versions. Linux runs race checks for orchestration and vulnerability
checks for both platforms; this does not imply Linux recording support. New Go
tags start at `v0.1.0`; the release workflow validates ancestry and checks, then
creates a draft GitHub release. Do not create a tag before live acceptance: a tag
itself makes a Go module fetchable even while the GitHub release is a draft.

Protocol reference: [Riot Replay API documentation](https://developer.riotgames.com/docs/lol#game-client-api_replay-api).
