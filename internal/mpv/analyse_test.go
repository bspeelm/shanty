package mpv

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func recorded(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(measured)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTheRecordedFilterOutputParsesIntoBlocks(t *testing.T) {
	blocks := parseBlocks(strings.NewReader(recorded(t)))
	if len(blocks) != 26 {
		t.Fatalf("got %d blocks, want 26", len(blocks))
	}
	first := blocks[0]
	if first.At != 0 || first.Samples != 1024 || len(first.Channels) != 2 {
		t.Fatalf("the first block is %+v", first)
	}
	want := Channel{Loudness: -11.617140, Difference: 1225.083634, Crossings: 20}
	for i, c := range first.Channels {
		if c != want {
			t.Errorf("channel %d of the first block is %+v, want %+v", i+1, c, want)
		}
	}
	if got := blocks[21].At; got != 487619*time.Microsecond {
		t.Errorf("block 21 starts at %s, want 487.619ms", got)
	}
}

func TestSilenceParsesAsNegativeInfinity(t *testing.T) {
	blocks := parseBlocks(strings.NewReader(recorded(t)))
	for _, c := range blocks[1].Channels {
		if !math.IsInf(c.Loudness, -1) {
			t.Errorf("a silent block has loudness %v", c.Loudness)
		}
	}
}

func TestTheShortLastBlockKeepsItsLengthAndTime(t *testing.T) {
	blocks := parseBlocks(strings.NewReader(recorded(t)))
	last := blocks[len(blocks)-1]
	if last.Samples != 1016 {
		t.Errorf("the last block holds %v samples, want 1016", last.Samples)
	}
	if want := 29976961 * time.Microsecond; last.At != want {
		t.Errorf("the last block starts at %s, want %s", last.At, want)
	}
}

func TestAMalformedLineIsSkippedAndTheBlockKept(t *testing.T) {
	text := strings.Join([]string{
		"AO: [null] 44100Hz stereo 2ch s16",
		"frame:0    pts:0       pts_time:0",
		"lavfi.astats.1.RMS_level=-20.0",
		"lavfi.astats.1.RMS_level=loud",
		"lavfi.astats.x.RMS_level=-3",
		"lavfi.astats.1.Zero_crossings",
		"garbage",
		"lavfi.astats.Overall.Number_of_samples=1024",
		"frame:1 pts:1024 pts_time:none",
		"lavfi.astats.1.RMS_level=-1",
		"lavfi.astats.Overall.Number_of_samples=1024",
	}, "\n")
	blocks := parseBlocks(strings.NewReader(text))
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want the one well-formed block: %+v", len(blocks), blocks)
	}
	if got := blocks[0].Channels; len(got) != 1 || got[0].Loudness != -20 {
		t.Errorf("the block's channels are %+v", got)
	}
}

// analysing points Options at the re-executed test binary as an analysing
// mpv, and returns the directory it reports into.
func analysing(t *testing.T, extraEnv ...string) (Options, string) {
	t.Helper()
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(stubEnv, "1")
	t.Setenv(reportEnv, dir)
	for _, kv := range extraEnv {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	return Options{Binary: self, Socket: filepath.Join(shortDir(t), "run", "mpv.sock")}, dir
}

func TestAnalyseReturnsEveryBlockWithItsFormat(t *testing.T) {
	opt, dir := analysing(t)
	blocks, err := Analyse(t.Context(), opt, streamURL())
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 26 {
		t.Fatalf("got %d blocks, want 26", len(blocks))
	}
	for _, b := range blocks {
		if b.Rate != 44100 || b.Scale != 1<<15 {
			t.Fatalf("a block has rate %d and scale %v, want 44100 and 32768", b.Rate, b.Scale)
		}
	}
	argv := report(t, dir, argvFile)
	for _, want := range []string{"--no-config", "--ao=null", "--ao-null-untimed=yes", "--input-ipc-server=" + opt.Socket + analyseSuffix} {
		if !strings.Contains(argv, want) {
			t.Errorf("the analysing mpv was not given %s:\n%s", want, argv)
		}
	}
	if _, err := os.Stat(opt.Socket + analyseSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the analysing socket is left behind: %v", err)
	}
}

func TestAnalysisHygiene(t *testing.T) {
	opt, dir := analysing(t, "SHANTY_TEST_CANARY=not-a-credential")
	if _, err := Analyse(t.Context(), opt, streamURL()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(commands(t, dir), fakeToken) {
		t.Fatal("the stream URL never reached the analysing mpv, so this test proves nothing about where it did not go")
	}
	argv, environ := report(t, dir, argvFile), report(t, dir, environFile)
	if !strings.Contains(environ, "SHANTY_TEST_CANARY=not-a-credential") {
		t.Fatal("the canary is missing from the child's environment; this test is not searching anything")
	}
	for _, secret := range []string{fakeToken, fakeSalt, "stream.view"} {
		if strings.Contains(argv, secret) {
			t.Errorf("the analysing mpv's argv carries %q:\n%s", secret, argv)
		}
		if strings.Contains(environ, secret) {
			t.Errorf("the analysing mpv's environment carries %q", secret)
		}
	}
}

func TestATrackMpvCannotReadIsAnError(t *testing.T) {
	opt, _ := analysing(t, endEnv+"=error")
	blocks, err := Analyse(t.Context(), opt, streamURL())
	if err == nil || !strings.Contains(err.Error(), "error") {
		t.Fatalf("got %d blocks and error %v, want the reason mpv gave", len(blocks), err)
	}
}

func TestCancellingAnAnalysisStopsItsMpv(t *testing.T) {
	opt, dir := analysing(t, endEnv+"=never")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := Analyse(ctx, opt, streamURL())
		done <- err
	}()
	for !strings.Contains(commands(t, dir), "loadfile") {
		waitABit()
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("a cancelled analysis returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled analysis did not return")
	}
}

func TestAnAnalysisSocketTooLongIsRefused(t *testing.T) {
	_, err := Analyse(t.Context(), Options{Socket: "/" + strings.Repeat("s", 200)}, streamURL())
	if err == nil || !strings.Contains(err.Error(), "characters") {
		t.Errorf("got %v, want the path refused for its length", err)
	}
}
