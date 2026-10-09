# Security policy

The Go implementation is an experimental alpha awaiting real-game acceptance.
Historical TypeScript/npm releases are retained in Git history; this migration
does not provide maintenance or compatibility for them.

Please report vulnerabilities using GitHub's private vulnerability reporting
feature if it is available. Otherwise, open an issue requesting a private contact
without publishing exploit details, credentials, or replay contents.

The recorder accepts local executable and replay paths and can send keyboard input
to its owned game window. Use trusted executables and inputs. It does not download
executables or automate Riot password login. The optional discovery package
handles caller-provided developer API keys; the observer package downloads
opaque game data from an explicitly configured endpoint and provides an HTTP
handler. Constructors do not start a listener or game process.

The Replay API client accepts numeric loopback origins only and rejects redirects
and proxy routing. Its default per-client certificate exception supports the local
game endpoint. `Config.StrictTLS` requires OS trust instead. Global TLS settings
are never changed.

Discovery keeps the API key in an HTTP header and disables redirects. Observer
traffic may use plaintext HTTP where that is the observed server protocol;
it must not carry a Riot API key. Caller-supplied transports and endpoints are
trusted configuration. Do not publish API keys, observer metadata, encryption
keys, or captured game data. Files may contain sensitive match information;
keep archives in a private directory. Restrictive Go file modes do not configure
Windows ACLs.

Serve archives only to trusted local consumers. The example binds numeric
loopback and supports cancellation; the HTTP handler itself is not an
authentication layer. Do not expose it to a public network without independent
access controls. Archives should be owned by a trusted user and not concurrently
modified. Manifest and path validation cannot protect against another process
with permission to replace files while they are being served.
