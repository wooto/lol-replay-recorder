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
- A native completion retained the output path and final current time but reset
  `endTime` to -1. Completion must still prove target camera and decoded media.
- Frame-enforced mode produced shortened videos despite whole-game API progress.
  An isolated 30-second real-time capture produced a 29.994666-second VP9 WebM at
  1280x720/30 FPS. Real-time mode is therefore used; whole-output validation remains
  required rather than trusting API progress.

TDD regressions cover ignored API selection, death/respawn, unconfirmed or stale
death data, same-target re-verification, encoder clock ownership, completion
sentinels, and camera loss at completion. The live FULL acceptance status remains
pending until a whole-game output passes decoding and duration validation.
