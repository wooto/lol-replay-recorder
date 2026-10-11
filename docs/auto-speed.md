# Automatic replay speed

`Config.AutoSpeed` defaults to false. With it enabled the existing capture loop
uses the Replay API playback speed while native recording has frame enforcement
disabled. No second camera/controller goroutine is started.

Only verified target deaths are compressed. The first five observed game seconds
stay at 1x, then a valid remaining respawn timer greater than three seconds permits
4x. Restoration starts before the three-second respawn boundary, reserving a
500ms controller budget and polling latency at 4x. Camera recovery restores 1x
before blocking work. The last three seconds before respawn and completion stay at 1x.
Missing/stale target status restores 1x; failed writes/readbacks fail the recording.
Opening acceleration is intentionally unavailable because no reliable attack or
enemy proximity telemetry is exposed by the documented APIs inspected so far.

`Result.DurationSeconds` still represents game-time coverage. Auto-speed results
also return `ExpectedMediaSeconds` and contiguous `SpeedSegments` using the speed
application POST response timestamp, validated by
`ExpectedMediaDuration`. Final media probing uses that expected media duration
without relaxing existing decoded-frame, packet coverage, or resolution checks.

Live capability probe on client 16.20: 180–210 game seconds, with playback speed
4x between about 190 and 200, produced 22.378666 seconds of WebM. Native recording
`replaySpeed=1` did not prevent playback speed changes. This probe does not establish
automatic death detection, camera stability or full-match completion.
