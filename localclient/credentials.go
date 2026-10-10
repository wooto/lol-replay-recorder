// Package localclient reads credentials of already running Riot/League clients.
package localclient

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Credentials are short-lived local remoting credentials, not Riot Web API keys.
// Never log Token or the original process command line.
type Credentials struct {
	Port  int
	Token string
}

func (c Credentials) String() string {
	return fmt.Sprintf("local client on port %d (credentials redacted)", c.Port)
}
func (c Credentials) GoString() string { return c.String() }
func (c Credentials) BaseURL() string  { return fmt.Sprintf("https://127.0.0.1:%d", c.Port) }
func ReadLockfile(path string) (Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, errors.New("cannot read client lockfile")
	}
	p := strings.Split(strings.TrimSpace(string(data)), ":")
	if len(p) != 5 || p[0] == "" || p[4] != "https" {
		return Credentials{}, errors.New("invalid client lockfile")
	}
	pid, e := strconv.Atoi(p[1])
	if e != nil || pid <= 0 {
		return Credentials{}, errors.New("invalid client lockfile process")
	}
	return credentials(p[2], p[3])
}

// FromCommandLine reads exact credential flags from a running process command
// line. riotClient selects the LeagueClientUx --riotclient-* flags; false selects
// that process's own --app-port/--remoting-auth-token credentials.
func FromCommandLine(line string, riotClient bool) (Credentials, error) {
	portFlag, tokenFlag := "app-port", "remoting-auth-token"
	if riotClient {
		portFlag, tokenFlag = "riotclient-app-port", "riotclient-auth-token"
	}
	read := func(flag string) (string, error) {
		re := regexp.MustCompile(`(?:^|\s)"?--` + flag + `=(?:"([^"]+)"|([^\s"]+))"?`)
		matches := re.FindAllStringSubmatch(line, -1)
		if len(matches) != 1 {
			return "", errors.New("missing or duplicate local credential flag")
		}
		if matches[0][1] != "" {
			return matches[0][1], nil
		}
		return matches[0][2], nil
	}
	port, err := read(portFlag)
	if err != nil {
		return Credentials{}, err
	}
	token, err := read(tokenFlag)
	if err != nil {
		return Credentials{}, err
	}
	return credentials(port, token)
}
func credentials(port, token string) (Credentials, error) {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return Credentials{}, errors.New("invalid local client credentials")
	}
	return Credentials{Port: n, Token: token}, nil
}
