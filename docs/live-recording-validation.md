# Windows native recording validation

Reference behavior was checked against the installed patch 16.20.824.8524 Replay
API OpenAPI specification and [Riot's Replay API documentation](https://developer.riotgames.com/docs/lol#game-client-api_replay-api).
The pinned [League Director recording flow](https://github.com/RiotGames/leaguedirector/blob/6ddb4ceb45974d6518b43d14522fa9b5ea422139/leaguedirector/app.py#L478-L500)
starts playback before enabling recording. Its older models do not cover the
current selection-name sequence, so the running client's schema is authoritative
for that track.

Observed during local testing on October 9, 2026:

- Windows refused direct game executable launch, while the logged-in League
  client's replay-watch operation successfully launched an owned replay game.
  `Config.LaunchReplay` supports caller-owned client integrations without a core
  LCU or authentication dependency.
- Winsock reports closed-port errors as 10061, not Go's synthetic Windows
  `syscall.ECONNREFUSED`. A real closed-listener regression covers preflight.
- Champion selection was unavailable at exact zero, available at 0.1 seconds,
  and could clear after encoder seeks or champion death. A constant selection
  track and bounded re-verification preserve the target through those transitions.
- A name/attachment-only sequence could put the camera inside terrain even while
  its identity checks passed. Short captures at combat and death verified an
  elevated selection offset `(0, 1492.267578125, -1006.5472412109375)` with rotation
  `(0, 56, 0)` in the API's `fps` mode. The recorder applies constant offset and
  rotation tracks, verifies their readback before capture, and rejects drift.
  These values reproduce the tested normal spectator angle; they do not restore
  the original player's mouse-driven camera or establish compatibility with all maps.
- A native completion retained the output path and final current time but reset
  `endTime` to -1. Completion must still prove target camera and decoded media.
- Frame-enforced mode produced shortened videos despite whole-game API progress.
  An isolated 30-second real-time capture produced a 29.994666-second VP9 WebM at
  1280x720/30 FPS. Real-time mode is therefore used; whole-output validation remains
  required rather than trusting API progress.
- FPS-mode real-time recording with `startTime=0` began at HUD 00:05 and ended
  its video packets about five seconds before the requested duration, while
  audio retained the full container duration. A five-second native pre-roll
  (`startTime=-5`, echoed by the API) produced HUD 00:00 at the first frame and
  29.898 seconds of video coverage in a 30.001333-second, 30-second test.
  The installed schema accepts a floating-point start time without a lower bound;
  Riot's public documentation does not promise negative pre-roll semantics.
  This compensation is empirical and patch-dependent. Output validation checks
  actual video packet bounds and fully decodes video, rather than trusting audio
  length or dividing the frame count by FPS (native WebM is variable-rate).

TDD regressions cover ignored API selection, death/respawn, unconfirmed or stale
death data, same-target re-verification, encoder clock ownership, completion
sentinels, and camera loss at completion.
Camera-profile regressions also cover accepted-but-ignored settings, lost offset
during capture, and persistent elevated tracks across encoder seeks.
Further regressions reject short video hidden by full audio, preserve the native
pre-roll, and accept ffprobe's empty WebM program section. Packet JSON is consumed
one record at a time so long recordings do not accumulate all packet metadata.

## FULL acceptance result

On October 9, 2026, one compatible KR local replay passed `RecordFull` on Windows
amd64 with patch 16.20.824.8524. The client used windowed 1280×720 and requested
30 FPS. Launch used the caller-owned hook through an already authenticated client.
The recording core did not log in, download the replay, or depend on LCU.

- Replay length: 498.412384 seconds; WebM container: 498.304000 seconds.
- First video packet: 0.035000 seconds; last packet end: 498.276000 seconds.
- 13,350 video packets; VP9 video and Vorbis audio. Native real-time output is
  variable-rate, so the requested 30 FPS is not a fixed count-per-second guarantee.
- The first captured frame showed HUD 00:00; combat, death, and victory samples
  retained the selected player's elevated camera. Live API monitoring also
  reverified identity and the camera profile through death and respawn.
- Full video decoding and packet-range validation succeeded. The owned replay
  process exited during cleanup; no active game remained.

This verifies one profile and replay, not every patch, game mode, machine, or the
default 1920×1080/60 FPS profile. Observer-archive playback remains unverified.
