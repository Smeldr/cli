package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// streamServer serves a scripted stream: each connection writes the next
// script's lines and then closes cleanly (EOF), or fails with a status.
type streamServer struct {
	mu      sync.Mutex
	scripts []streamScript
	queries []string
	auth    []string
}

type streamScript struct {
	status int
	lines  []string
	hangup bool // drop the connection mid-body (an error, not a clean close)
}

func (s *streamServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.queries = append(s.queries, r.URL.RawQuery)
	s.auth = append(s.auth, r.Header.Get("Authorization"))
	var sc streamScript
	if len(s.scripts) > 0 {
		sc = s.scripts[0]
		s.scripts = s.scripts[1:]
	} else {
		sc = streamScript{status: http.StatusUnauthorized, lines: []string{"stop"}}
	}
	s.mu.Unlock()
	if sc.status != 0 && sc.status != http.StatusOK {
		http.Error(w, strings.Join(sc.lines, "\n"), sc.status)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	for _, l := range sc.lines {
		fmt.Fprintln(w, l)
	}
	if sc.hangup {
		if hj, ok := w.(http.Hijacker); ok {
			w.(http.Flusher).Flush()
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}
}

func noSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var waits []time.Duration
	prev := watchSleep
	watchSleep = func(_ context.Context, d time.Duration) { waits = append(waits, d) }
	t.Cleanup(func() { watchSleep = prev })
	return &waits
}

// A clean close (a server restart) and an error both reconnect with ?after=
// the last event; duplicates and heartbeats are not printed; a refusal ends
// the watch with an error.
func TestWatch_ReconnectsAfterCleanCloseAndError(t *testing.T) {
	waits := noSleep(t)
	srv := &streamServer{scripts: []streamScript{
		{lines: []string{`{"type":"ping"}`, `{"id":"e1","event":"task.created"}`, `{"id":"e2","event":"task.transitioned"}`}},
		{lines: []string{`{"id":"e2","event":"task.transitioned"}`, `{"type":"caught_up"}`, `{"id":"e3","event":"signal.created"}`}, hangup: true},
		{status: http.StatusBadGateway},
		{lines: []string{`not json`, `{"id":"e4","event":"task.updated"}`}},
		{status: http.StatusUnauthorized, lines: []string{`{"error":"unauthorized"}`}},
	}}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	var out, errOut bytes.Buffer
	err := watch(context.Background(), Config{ForgeURL: ts.URL, Token: "tok"}, watchOptions{subscription: "N3"}, &out, &errOut)
	var refused errRefused
	if !errors.As(err, &refused) || refused.status != http.StatusUnauthorized {
		t.Fatalf("watch ended with %v, want the 401 refusal", err)
	}
	got := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{`{"id":"e1","event":"task.created"}`, `{"id":"e2","event":"task.transitioned"}`, `{"id":"e3","event":"signal.created"}`, `{"id":"e4","event":"task.updated"}`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("stdout = %q", got)
	}
	if !strings.Contains(errOut.String(), `"caught_up"`) || !strings.Contains(errOut.String(), "not JSON") {
		t.Errorf("stderr = %q", errOut.String())
	}
	wantQ := []string{"subscription=N3", "after=e2&subscription=N3", "after=e3&subscription=N3", "after=e3&subscription=N3", "after=e4&subscription=N3"}
	if strings.Join(srv.queries, "|") != strings.Join(wantQ, "|") {
		t.Errorf("queries = %q, want %q", srv.queries, wantQ)
	}
	if srv.auth[0] != "Bearer tok" {
		t.Errorf("auth = %q", srv.auth[0])
	}
	// The backoff resets after each accepted connection (the clean close, the
	// hangup and the last stream) and doubles after the 502.
	if fmt.Sprint(*waits) != "[1s 1s 2s 1s]" {
		t.Errorf("waits = %v", *waits)
	}
}

func TestWatch_StopsOnCancelAndCapsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var waits []time.Duration
	prev := watchSleep
	watchSleep = func(_ context.Context, d time.Duration) {
		waits = append(waits, d)
		if len(waits) == 8 {
			cancel()
		}
	}
	defer func() { watchSleep = prev }()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	var out, errOut bytes.Buffer
	if err := watch(ctx, Config{ForgeURL: ts.URL}, watchOptions{channel: "core,architect", includeOwn: true}, &out, &errOut); err != nil {
		t.Fatalf("watch = %v", err)
	}
	if waits[len(waits)-1] != watchMaxBackoff {
		t.Errorf("waits = %v, want the cap reached", waits)
	}
	// A server that cannot be reached is retried, not refused.
	ctx2, cancel2 := context.WithCancel(context.Background())
	watchSleep = func(context.Context, time.Duration) { cancel2() }
	if err := watch(ctx2, Config{ForgeURL: "http://127.0.0.1:1"}, watchOptions{}, &out, &errOut); err != nil {
		t.Errorf("unreachable = %v", err)
	}
	if err := watch(context.Background(), Config{ForgeURL: "http://bad\x7f"}, watchOptions{}, &out, &errOut); err == nil {
		t.Error("a bad URL was retried")
	}
	real := prev
	c3, cancel3 := context.WithCancel(context.Background())
	cancel3()
	start := time.Now()
	real(c3, time.Hour)
	if time.Since(start) > time.Second {
		t.Error("watchSleep ignored the context")
	}
	real(context.Background(), time.Millisecond)
}

func TestStreamURLAndSeen(t *testing.T) {
	if got := streamURL("https://x", watchOptions{}, ""); got != "https://x/_events/stream" {
		t.Errorf("bare = %q", got)
	}
	if got := streamURL("https://x", watchOptions{channel: "core", includeOwn: true}, "e9"); got != "https://x/_events/stream?after=e9&channel=core&include_own=true" {
		t.Errorf("channel = %q", got)
	}
	s := newSeenIDs()
	if !s.add("a") || s.add("a") {
		t.Error("dedupe")
	}
	for i := 0; i < watchSeenSize; i++ {
		s.add(fmt.Sprint(i))
	}
	if !s.add("a") {
		t.Error("an ID older than the window was still held")
	}
}

func TestParseWatchArgs(t *testing.T) {
	opts, err := parseWatchArgs([]string{"--subscription", "N3", "--after", "e1", "--include-own"})
	if err != nil || opts != (watchOptions{subscription: "N3", after: "e1", includeOwn: true}) {
		t.Errorf("opts = %+v, %v", opts, err)
	}
	for _, args := range [][]string{
		{"--subscription", "N3", "--channel", "core"},
		{"pos"},
		{"--nope", "x"},
		{"--include-own=maybe"},
		{"--after"},
	} {
		if _, err := parseWatchArgs(args); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
}

// A write failure on stdout ends the watch.
func TestWatch_StdoutFailure(t *testing.T) {
	noSleep(t)
	ts := httptest.NewServer(&streamServer{scripts: []streamScript{{lines: []string{`{"id":"e1"}`}}}})
	defer ts.Close()
	var errOut bytes.Buffer
	if err := watch(context.Background(), Config{ForgeURL: ts.URL}, watchOptions{}, failWriter{}, &errOut); err == nil {
		t.Error("a stdout failure did not end the watch")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }
