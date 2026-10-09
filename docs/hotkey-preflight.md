# Spectator hotkey preflight

`Config.Hotkeys` optionally integrates effective player-slot settings with
`RecordFull`. Checks run after excluding an existing game, under the recorder's
process lease, before launching a new replay. Keys use the same ORDER then CHAOS
slot order as `Config.SelectionKeys`.

The adapter implements `HotkeySettings`:

- `Read(ctx)` returns ten effective Windows virtual-key codes, or an error when
  the current client's spectator bindings cannot be determined. Missing,
  duplicate, or out-of-range slot bindings are rejected before any change.
  Configured desired bindings must also be distinct and nonzero.
  A successful settings request without a
  spectator group is insufficient; do not substitute guessed defaults.
- `Backup(ctx)` durably backs up the original settings before any mutation.
  A backup error prevents `Apply` and replay launch.
- `Apply(ctx, keys)` changes only those spectator bindings and makes them effective
  for the next replay launch. Handle client reload requirements inside the adapter.
  Preserve unrelated settings and protect against concurrent writers.

With `ConfigureHotkeys: false` (default), a mismatch returns `ErrHotkeySettings`
without modifying settings. With `ConfigureHotkeys: true`, a mismatch triggers
backup, apply, and a second read. The second read must match before launch; a
successful write response alone is not enough. Auto-configuration requires an
adapter. Errors retain the `validate` stage and underlying cancellation causes.
After a settings failure the adapter's backup remains available; there is no
automatic rollback of a partial external write.

```go
r, err := recorder.New(recorder.Config{
    Hotkeys: supportedClientSettings, // your HotkeySettings implementation
    ConfigureHotkeys: true,          // explicitly enable backup/correction
})
```

No built-in LCU or config-file adapter is shipped. The installed patch's LCU
input settings/schema did not expose spectator slot bindings, so unsupported
event names are not written to `input.ini`. Without an adapter, the prior
unchecked settings behavior remains. This integration is not a claim that native
spectator focus or synthetic input works on the installed client: the current
Windows computer-use input still fails and native player focus remains unverified.
The existing FPS camera/recording behavior is unchanged.

Riot documents double presses of `1–5` / `Q,W,E,R,T` for champion locking in its
[Replays FAQ](https://support.riotgames.com/en-us/league-of-legends/gameplay/replays-faq-pro-tips).
Settings verification and actual target-camera verification are separate checks.

## Target slot from the champion roster

The recorder already uses `allPlayers` team membership and original order within
each team to locate the full target Riot ID. ORDER slots map to configured keys
0–4 (default `1–5`), CHAOS slots to keys 5–9 (default `Q,W,E,R,T`). It counts each
team separately, so interleaved blue/red arrays do not shift the player's slot.
It does not sort by champion or player name, or identify players by champion
alone. RecordFull regression tests cover both teams and interleaved rosters.

In the currently opened replay, target `헬로지토#HELLO` played Kayle and appeared
first in CHAOS, yielding default key `Q`. This is a roster-to-key inference,
not proof that the game accepted the key or that a remapped key remains `Q`.
Native camera/target readback must still establish successful focusing.
