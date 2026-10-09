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
  `(0, 56, 0)` in the API's `fps` mode. Alpha.2 applied constant offset and
  rotation tracks, verified their readback before capture, and rejected drift.
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

## Alpha.2 FULL acceptance result

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

## Alpha.3 smooth selected-player following

Alpha.3 replaces the constant selection-offset track with
short linear camera keyframes while retaining selection-name and rotation tracks. It derives target coordinates from
Replay API `cameraPosition - selectionOffset`, then eases horizontal camera
movement with a 180 ms time constant, then asks the client to interpolate the
offset between frames. Height and viewing angle remain fixed. FPS movement and
look speeds are zero, while axes remain unlocked: native tests showed that axis
locks freeze the camera in world space even when a target remains selected.
All five settings are verified before capture and during recording.
The camera loop runs at most 50 ms between iterations; progress notifications
retain the configured poll interval. API requests and callbacks add to that time.

The recorder verifies offset readback before interpreting target movement. A
large target jump with an unchanged acknowledged offset resets the follower;
offset drift cannot masquerade as a teleport. First initialization, encoder time
rewind, and target reacquisition reinitialize camera motion. Missing coordinates,
ignored keyframes, and unexpected camera changes fail without a FULL result.
When no update is sent within native precision tolerance, the actual readback
remains the acknowledged offset.

This is library-controlled easing for the selected player, not native directed
spectator-camera behavior. Early direct-offset capture attempts stopped on
readback mismatches and were retained as partial outputs. Native keyframe probes
then applied 239 fractional horizontal-offset updates across game-time 70–100
and 170–200 seconds, with no endpoint mismatches after interpolation. These
short probes establish camera-control behavior, not FULL recording acceptance.

The exact recording source at `89db3b9` subsequently passed a complete native
`RecordFull` call on the same Windows/patch/replay/profile as alpha.2, with no
external camera override in the caller's launch hook:

- Replay: 498.412384 seconds; WebM: 498.406656 seconds; size: 337,721,720 bytes.
- First video packet: 0.037000 seconds; final packet end: 498.318000 seconds.
- 13,444 video packets; VP9/Vorbis at windowed 1280×720, requested 30 FPS (VFR).
- Native camera acknowledgment, target identity, input controls, death/respawn,
  full video decoding, packet coverage, and owned-process cleanup all passed.
- Inspected frames show HUD 00:00, combat at 03:00, target death at 04:04, and
  victory at 08:17 with the selected player's elevated camera.

This verifies library-controlled smooth following on one compatible local
replay. Native directed spectator-camera switching and the original player's
mouse-driven view are not implemented.
