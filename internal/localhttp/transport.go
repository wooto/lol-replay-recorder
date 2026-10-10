package localhttp

import (
	"net"
	"net/http"
	"time"
)

// CloneTransport isolates connection pools and TLS settings. Applications may
// replace http.DefaultTransport with a custom RoundTripper; that must not make
// construction panic or route local credentials through an unknown transport.
func CloneTransport() *http.Transport {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		return transport.Clone()
	}
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}
