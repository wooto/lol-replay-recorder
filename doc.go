// Package recorder launches local League of Legends replays on Windows and
// records a complete match with the camera locked to a specified player.
//
// Recording requires an interactive Windows desktop, a compatible League
// installation and replay, and EnableReplayApi=1 in the game's configuration.
// It does not download replays, log into Riot, or modify game configuration.
//
// The recorder and local API clients use isolated HTTP connection pools with
// proxies and redirects disabled. A host application's custom DefaultTransport
// does not intercept local client credentials. Local API cancellation and
// timeout errors support errors.Is with context.Canceled and
// context.DeadlineExceeded, including cancellation during response reads.
package recorder
