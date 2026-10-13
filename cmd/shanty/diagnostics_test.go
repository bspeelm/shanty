package main

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// mpv names the stream it is opening, and a stream URL carries the credential:
// "[ytdl_hook] ERROR: Unsupported URL: https://…?apiKey=…" is a real line from
// a real failure. Keeping mpv's words is only safe if that never survives.
func TestWhatMpvSaidNeverCarriesTheCredential(t *testing.T) {
	a, _, _ := wired(t)
	stream := a.client.StreamURL("tr-1")
	parsed, err := url.Parse(stream)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if len(query) == 0 {
		t.Fatal("the stream URL has no query, so this test proves nothing")
	}

	d := &diagnostics{}
	fmt.Fprintf(d, "[ytdl_hook] ERROR: Unsupported URL: %s\n", stream)
	held := strings.Join(d.said(), "\n")

	for key, values := range query {
		for _, value := range values {
			// Values of three characters or fewer are skipped: they collide
			// with ordinary words by accident and prove nothing either way.
			if len(value) > 3 && strings.Contains(held, value) {
				t.Errorf("the line kept carries %s=%s from the stream URL:\n%s", key, value, held)
			}
		}
	}
	if !strings.Contains(held, "Unsupported URL") {
		t.Errorf("redacting threw away the message itself:\n%s", held)
	}
	if !strings.Contains(held, parsed.Host) {
		t.Errorf("redacting threw away the host, which is not secret:\n%s", held)
	}
}

// Only the end of what mpv said is wanted. Its account of a failure is the
// last thing it says, and a whole session's output does not belong in a
// message on screen.
func TestOnlyTheLastFewThingsMpvSaidAreKept(t *testing.T) {
	d := &diagnostics{}
	for i := range kept * 3 {
		fmt.Fprintf(d, "line %d\n", i)
	}
	held := d.said()
	if len(held) != kept {
		t.Fatalf("holding %d lines, want %d", len(held), kept)
	}
	if want := fmt.Sprintf("line %d", kept*3-1); held[len(held)-1] != want {
		t.Errorf("the newest line is %q, want %q", held[len(held)-1], want)
	}
}

// mpv writes its status line with a carriage return and no newline. A reader
// that waited for a newline would hold the whole of it in memory for as long
// as the music played.
func TestMpvsStatusLineDoesNotGrowInMemory(t *testing.T) {
	d := &diagnostics{}
	for range 200 {
		fmt.Fprintf(d, "A: 00:00:01 / 00:04:05 (1%%)\r")
	}
	if len(d.part) > partial {
		t.Errorf("%d bytes are held waiting for a newline", len(d.part))
	}
	if len(d.said()) != kept {
		t.Errorf("a carriage return did not end a line: %v", d.said())
	}
}

// A run of bytes with no line ending at all is dropped rather than kept.
func TestOutputWithNoLineEndingIsNotHeldForever(t *testing.T) {
	d := &diagnostics{}
	for range 40 {
		fmt.Fprint(d, strings.Repeat("x", 1024))
	}
	if len(d.part) > partial {
		t.Errorf("%d bytes are held with no line ending in sight", len(d.part))
	}
}
