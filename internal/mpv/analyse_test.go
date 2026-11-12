package mpv

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
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

func TestAnalyseReturnsEveryBlockWithItsWholeSoundAndFourBands(t *testing.T) {
	opt, dir := analysing(t)
	blocks, err := Analyse(t.Context(), opt, streamURL())
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 11 {
		t.Fatalf("got %d blocks, want 11", len(blocks))
	}
	for _, b := range blocks {
		if b.Rate != 48000 || b.Scale != 1 || len(b.Channels) != 1 || len(b.Bands) != 4 {
			t.Fatalf("a block has rate %d, scale %v, %d channels and %d bands, want 48000, 1, 1 and 4",
				b.Rate, b.Scale, len(b.Channels), len(b.Bands))
		}
	}
	first := blocks[0]
	if want := (Channel{Loudness: -6.860854, Difference: 0.003394, Crossings: 3}); first.Channels[0] != want {
		t.Errorf("the whole sound of the first block is %+v, want %+v", first.Channels[0], want)
	}
	if want := []float64{-7.215836, -18.148681, -38.191460, -75.977048}; !slices.Equal(first.Bands, want) {
		t.Errorf("the bands of the first block are %v, want %v", first.Bands, want)
	}
	if b := blocks[3]; !math.IsInf(b.Channels[0].Loudness, -1) || b.Bands[0] > -100 {
		t.Errorf("the silent block at %s measured %+v and bands %v", b.At, b.Channels[0], b.Bands)
	}
	before, thump := blocks[6], blocks[7]
	if thump.At != 490667*time.Microsecond || thump.Bands[0]-before.Bands[0] < 60 {
		t.Errorf("the thump at half a second rose from %v dB to %v dB in the lowest band, in the block at %s",
			before.Bands[0], thump.Bands[0], thump.At)
	}
	if last := blocks[len(blocks)-1]; last.Samples != 224 {
		t.Errorf("the last block holds %v samples, want 224", last.Samples)
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

func TestTheAnalysingFilterMeasuresTheWholeSoundAndFourBandsAtAFixedRate(t *testing.T) {
	for _, want := range []string{"aresample=48000", "channel_layouts=mono", "asetnsamples=n=1024", "amerge=inputs=5"} {
		if !strings.Contains(analyseFilter, want) {
			t.Errorf("the analysing filter lacks %s:\n%s", want, analyseFilter)
		}
	}
}

func TestABlockMeasuredWithoutBandsKeepsItsChannelsAndHasNoBands(t *testing.T) {
	blocks := parseBlocks(strings.NewReader(recorded(t)))
	b := banded(blocks[0])
	if len(b.Channels) != 2 || len(b.Bands) != 0 || b.Rate != 48000 || b.Scale != 1 {
		t.Errorf("a two-channel block became %+v", b)
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
