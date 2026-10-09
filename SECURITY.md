# Security policy

The Go implementation is currently unreleased and awaiting real-game acceptance.
Historical TypeScript/npm releases are retained in Git history; this migration
does not provide maintenance or compatibility for them.

Please report vulnerabilities using GitHub's private vulnerability reporting
feature if it is available. Otherwise, open an issue requesting a private contact
without publishing exploit details, credentials, or replay contents.

The recorder accepts local executable and replay paths and can send keyboard input
to its owned game window. Use trusted executables and inputs. It does not download
executables or replays, handle Riot credentials, or expose an HTTP listener.

The Replay API client accepts numeric loopback origins only and rejects redirects
and proxy routing. Its default per-client certificate exception supports the local
game endpoint. `Config.StrictTLS` requires OS trust instead. Global TLS settings
are never changed.
