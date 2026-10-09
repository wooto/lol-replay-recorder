# Go alpha scope and acceptance

The user requested an MIT public Go replacement in
`wooto/lol-replay-recorder`, initially concentrating on Windows player-POV
recording. Subsequent research expanded the requested library to collecting
live observer data from game start and serving it later, independently of
OP.GG. The user explicitly authorized publishing the library and requested
recent implementation references and multiple Luna agents at max effort.

## Deliverables

- Preserve the implemented Windows local-ROFL `recorder.RecordFull` API.
- Add an experimental `observer` package for bounded opaque chunk/keyframe
  downloads, durable custom archives, cancellation, and explicit incomplete
  results. Never overwrite an existing archive destination.
- Mark an archive complete only when an observed game-end marker and the
  expected contiguous payload ranges agree. Reject contradictory positive
  end-chunk/end-keyframe identifiers, malformed IDs, missing payloads, and
  unsupported shapes. Ordinary metadata may be cached: a false `gameEnded` flag
  or lower `last*Id` does not override an explicit final chunk snapshot.
- Expose a caller-owned HTTP replay handler only for completed validated
  archives. Preserve original payload bytes and known metadata values; do not
  invent keyframe-to-chunk mappings or use per-IP playback counters.
- Optional `discovery` resolves a caller-selected Riot ID and waits for its
  observable active game through Riot APIs. Caller supplies its own API key.
  Respect rate-limit waits and fail promptly on authorization errors.
- Provide examples for local ROFL recording, observer collection, and a
  numeric-loopback replay server. No implicit account login, OP.GG integration,
  executable downloads, game-memory access, upload pipeline, or player crawler.
- Maintain standard-library-only Go dependencies, MIT attribution, and Windows
  minimum/stable Go checks plus Linux race/vulnerability checks.
- Publish `v0.1.0-alpha.1` on the public repository after tests and review.
  Stable publication remains gated on live acceptance. Do not label this alpha
  as current-patch KR-compatible, a FULL observer-to-video pipeline, or approved
  by Riot. Archive completion means observed protocol coverage, not proof of
  match-time-zero coverage or actual game-client playback.

## Verification

Use public HTTP/archive boundaries with deterministic fixtures to verify
payload preservation, live progress, late data availability, missing final
data, contradictory end markers, cancellations, input bounds, unsafe paths,
wrong game identity, method/route handling, and independent concurrent clients.
Check the complete module with vet, build, formatting, Windows tests, Linux
race checks, and vulnerability scanning. Confirm the published tag is fetchable
as a Go module. Live acceptance remains explicitly pending.
