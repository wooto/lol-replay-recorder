// Package riotapi exposes Riot Web API account lookup, current-game discovery
// and match history. discovery remains source-compatible for existing users.
package riotapi

import "github.com/wooto/lol-replay-recorder/discovery"

type Config = discovery.Config
type Client = discovery.Client
type Account = discovery.Account
type Game = discovery.Game
type Match = discovery.Match
type Participant = discovery.Participant
type StatusError = discovery.StatusError

var (
	ErrNoActiveGame    = discovery.ErrNoActiveGame
	ErrAccountNotFound = discovery.ErrAccountNotFound
	ErrInvalidResponse = discovery.ErrInvalidResponse
	ErrRedirect        = discovery.ErrRedirect
)

func New(config Config) (*Client, error) { return discovery.New(config) }
