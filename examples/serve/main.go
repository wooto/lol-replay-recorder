// Command serve exposes one completed experimental archive on numeric loopback.
// It does not start or authenticate a League game client.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/wooto/lol-replay-recorder/observer"
)

func main() {
	archive := flag.String("archive", "", "completed observer archive directory")
	bind := flag.String("bind", "127.0.0.1:0", "numeric loopback address; port 0 selects an available port")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, *archive, *bind); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, directory, bind string) error {
	host, _, err := net.SplitHostPort(bind)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("bind must be a numeric loopback host and port")
	}
	archive, err := observer.OpenArchive(directory)
	if err != nil {
		return err
	}
	handler, err := observer.NewReplayHandler(archive)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	fmt.Fprintf(os.Stderr, "Experimental replay endpoint: http://%s\nNo game client is launched.\n", listener.Addr())
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	select {
	case err := <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}
