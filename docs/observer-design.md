# Experimental observer archive design

This release separates three different responsibilities: a caller discovers an
active game, `observer` archives its opaque protocol data, and an HTTP handler
serves a completed archive. The Windows `recorder` independently records a
compatible local ROFL with a selected player's camera. An observer archive is
not a ROFL and the two paths are not an end-to-end automatic recorder yet.

## Sources inspected on 2026-10-09

- [Neeko-Server's March 2025 Spectator-v5 migration](https://github.com/Vidalee/Neeko-Server/commit/e6d9cef948ddc091429d58e7c0fa473e260d1361)
  and [June 2025 observer-host update](https://github.com/Vidalee/Neeko-Server/commit/83dd69133190f7efaf96c583226f2751ad6105ea).
  Its capture/server design informs the observed HTTP routes. Its
  [post-14.1 playback issue](https://github.com/Vidalee/Neeko-Server/issues/25)
  remains unresolved. A new commit or reachable host is not evidence of a
  working complete recording.
- [1lann/lol-replay](https://github.com/1lann/lol-replay) separates collection,
  storage, and HTTP playback in Go. Upstream and its 16 inspected forks had no
  post-2022 code changes. It is architectural reference, not a current dependency.
- [Clairvoyance](https://github.com/Emre-98/Clairvoyance), updated in October 2026,
  separates Windows capture, encoding, and game detection. It records an already
  running spectator window; it does not demonstrate raw observer archive replay.
  Its native Rust capture/encoder stack is not copied into this library.
- [lcu-gopher's September 2026 changes](https://github.com/Its-Haze/lcu-gopher/commit/81eb26e78fd543118fd894676263241ae24983b1)
  distinguish a running client from a logged-in session. This library similarly
  makes no claim that reaching an endpoint proves authentication or playback.
  That repository had no license file when inspected; its source is not copied
  or added as a dependency.
- [LeagueAkari's September 2026 replay adapter](https://github.com/LeagueAkari/LeagueAkari/blob/5109b2f7fcce6e02312e534cd1729ed0f2142b51/src/shared/http-api-axios-helper/league-client/replays.ts)
  demonstrates separate completed-replay download/watch calls. Those calls are
  not used as a substitute for live stream collection.

## Improvements over the historical collector

- Context-aware collection, lookup, rate-limit waits, and shutdown.
- Bounded response sizes and identifiers; malformed data fails explicitly.
- New destination directories and atomic writes rather than overwriting another
  archive or treating a started write as a completed write.
- Explicit incomplete results on cancellation, missing data, or failed requests.
- Completion checks over the expected chunk/keyframe set; no guessed successful
  result when data is missing. Contradictory positive final identifiers fail;
  lower cached `last*Id` values or a cached false `gameEnded` flag may lag the
  explicit final chunk snapshot and do not invalidate it.
- No unconditional dependency on `getLastKeyFrameInfo`, which the upstream
  issue reports as unavailable on changed servers.
- A caller-owned replay HTTP handler with no per-IP simulated progression.
  Completed archives expose their final availability snapshot; current-client
  compatibility with that offline representation needs live testing.
- No global TLS changes, OP.GG calls, game-memory access, account passwords,
  bundled executables, or additional Go dependencies.

## Acceptance still required

The KR server's `/observer-mode/rest/consumer/version` returned HTTP 200 and
`2.62.0` during a manual request on 2026-10-09. This checks only reachability.
Automated tests use simulated upstream data and do not constitute a real game
recording or official authorization.

A live acceptance must obtain an observable KR game early, collect its startup
and game data without gaps, save the announced end, restart the client against
the saved archive without the original server, and inspect beginning/end and
seek behavior. Camera tracking and video recording must then be tested separately.
Do not equate an archive's protocol-complete flag with verified FULL video.

Riot's [14.1 spectator changes](https://www.leagueoflegends.com/en-us/news/game-updates/patch-14-1-notes/)
remove viewing gameplay before the spectate request. Polling at match start is
not a guarantee of time-zero coverage. Riot's
[Private Replays delay announcement](https://x.com/LeagueOfLegends/status/2107875798825021469)
defers the announced restriction to 26.21 while preparing a creator/coach
solution. It does not guarantee raw observer access. The
[game-memory policy](https://support.riotgames.com/en-us/riot/performance/game-memory-access-removed-for-third-party-apps)
must not be interpreted as approving every internal HTTP endpoint.
