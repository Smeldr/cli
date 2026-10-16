package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"
)

// watchUsage is the help for the watch command.
const watchUsage = `smeldr-cli watch: print the event stream, one JSON line per event (Author role)

Usage:
  watch --subscription <address> [--after <event-id>]
  watch --channel <name[,name...]> [--after <event-id>] [--include-own]

--subscription delivers one of your subscriptions ("smeldr-cli subscription
list"); without --after it resumes from the subscription's saved cursor.
--channel routes as before (D97). With neither, you get all your
subscriptions.

Each event is printed to stdout as it arrives, so the command works as a
monitor. Heartbeats are not printed; caught_up and skipped go to stderr. A
dropped or closed connection is reconnected with ?after=<last event id>, with
a backoff of 1 s doubling to 30 s, and an event seen twice is printed once.
401, 403 and 404 end the command with exit 1. Ctrl-C stops it.

Needs smeldr.dev/core v1.162.0 or later on the server for --subscription.
`

// watchOptions is what to watch.
type watchOptions struct {
	subscription string
	channel      string
	after        string
	includeOwn   bool
}

// watchSleep waits d or until ctx ends; tests replace it.
var watchSleep = func(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// watchMaxBackoff caps the reconnect backoff.
const watchMaxBackoff = 30 * time.Second

// watchSeenSize is how many event IDs are kept to drop duplicates.
const watchSeenSize = 1024

// runWatchCommand runs watch until Ctrl-C or a refusal.
func runWatchCommand(args []string) {
	opts, err := parseWatchArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(os.Stdout, watchUsage)
		return
	}
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := watch(ctx, cfg, opts, os.Stdout, os.Stderr); err != nil {
		fatal("%v", err)
	}
}

// parseWatchArgs reads the watch flags.
func parseWatchArgs(args []string) (watchOptions, error) {
	c, err := splitArgs(args, map[string]bool{"include-own": true})
	if err != nil {
		return watchOptions{}, err
	}
	if err := c.only("watch", "subscription", "channel", "after", "include-own"); err != nil {
		return watchOptions{}, err
	}
	if len(c.pos) != 0 {
		return watchOptions{}, fmt.Errorf("watch takes flags only")
	}
	opts := watchOptions{subscription: c.flags["subscription"], channel: c.flags["channel"], after: c.flags["after"]}
	if opts.subscription != "" && opts.channel != "" {
		return watchOptions{}, fmt.Errorf("watch: --subscription and --channel cannot be combined")
	}
	if v, ok := c.flags["include-own"]; ok {
		if opts.includeOwn, err = boolFlag("include-own", v); err != nil {
			return watchOptions{}, err
		}
	}
	return opts, nil
}

// streamURL is the stream address for opts, resuming after after when set.
func streamURL(base string, opts watchOptions, after string) string {
	q := url.Values{}
	if opts.subscription != "" {
		q.Set("subscription", opts.subscription)
	}
	if opts.channel != "" {
		q.Set("channel", opts.channel)
	}
	if opts.includeOwn {
		q.Set("include_own", "true")
	}
	if after != "" {
		q.Set("after", after)
	}
	u := base + "/_events/stream"
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// seenIDs remembers the last watchSeenSize event IDs.
type seenIDs struct {
	ring []string
	set  map[string]bool
	next int
}

func newSeenIDs() *seenIDs {
	return &seenIDs{ring: make([]string, watchSeenSize), set: map[string]bool{}}
}

// add records id and reports whether it is new.
func (s *seenIDs) add(id string) bool {
	if s.set[id] {
		return false
	}
	if old := s.ring[s.next]; old != "" {
		delete(s.set, old)
	}
	s.ring[s.next] = id
	s.set[id] = true
	s.next = (s.next + 1) % len(s.ring)
	return true
}

// errRefused is a response that retrying does not heal.
type errRefused struct {
	status int
	body   string
}

func (e errRefused) Error() string {
	return fmt.Sprintf("the stream refused the connection (%d): %s", e.status, e.body)
}

// watch prints the stream to out until ctx ends or the server refuses. Every
// other end (an error, a clean close, a 5xx) is reconnected with
// ?after=<last event id>, after a backoff that resets once a connection is up.
func watch(ctx context.Context, cfg Config, opts watchOptions, out, errOut io.Writer) error {
	seen := newSeenIDs()
	last := opts.after
	backoff := time.Second
	for ctx.Err() == nil {
		connected, err := watchOnce(ctx, cfg, opts, last, seen, &last, out, errOut)
		var refused errRefused
		if errors.As(err, &refused) {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		if connected {
			backoff = time.Second
		}
		if err != nil {
			fmt.Fprintf(errOut, "watch: %v; reconnecting in %s\n", err, backoff)
		} else {
			fmt.Fprintf(errOut, "watch: the stream closed; reconnecting in %s\n", backoff)
		}
		watchSleep(ctx, backoff)
		backoff = min(backoff*2, watchMaxBackoff)
	}
	return nil
}

// watchOnce holds one connection, printing new events and moving *last. It
// reports whether the connection was accepted.
func watchOnce(ctx context.Context, cfg Config, opts watchOptions, after string, seen *seenIDs, last *string, out, errOut io.Writer) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL(cfg.ForgeURL, opts, after), nil)
	if err != nil {
		return false, errRefused{0, err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/x-ndjson")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusBadRequest:
			return false, errRefused{resp.StatusCode, strings.TrimSpace(string(body))}
		}
		return false, fmt.Errorf("the stream answered %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var env struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &env); err != nil {
			fmt.Fprintf(errOut, "watch: a line that is not JSON: %s\n", line)
			continue
		}
		switch {
		case env.Type == "ping":
			continue
		case env.Type != "" && env.ID == "":
			fmt.Fprintf(errOut, "%s\n", line)
			continue
		}
		if env.ID != "" {
			if !seen.add(env.ID) {
				continue
			}
			*last = env.ID
		}
		if _, err := fmt.Fprintf(out, "%s\n", line); err != nil {
			return true, errRefused{0, "stdout: " + err.Error()}
		}
	}
	return true, sc.Err()
}
