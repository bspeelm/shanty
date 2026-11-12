package mpv

import (
	"context"
	"errors"
	"fmt"
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

func TestAnalyseReturnsEveryBlockWithItsWholeSoundAndFiveBands(t *testing.T) {
	opt, dir := analysing(t)
	blocks, err := Analyse(t.Context(), opt, streamURL())
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 11 {
		t.Fatalf("got %d blocks, want 11", len(blocks))
	}
	for _, b := range blocks {
		if b.Rate != 48000 || b.Scale != 1 || len(b.Channels) != 2 || len(b.Bands) != 5 {
			t.Fatalf("a block has rate %d, scale %v, %d channels and %d bands, want 48000, 1, 2 and 5",
				b.Rate, b.Scale, len(b.Channels), len(b.Bands))
		}
	}
	first := blocks[0]
	want := Channel{Loudness: -9.871154, Difference: 0.0024, Crossings: 3 * 48000.0 / 44100}
	for i, c := range first.Channels {
		if c != want {
			t.Errorf("channel %d of the whole sound of the first block is %+v, want %+v", i+1, c, want)
		}
	}
	if want := []float64{-7.215836, -18.148681, -38.191460, -75.977048, -27.663779}; !slices.Equal(first.Bands, want) {
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

func TestTheAnalysingFilterMeasuresTheWholeSoundAndFiveBandsAtAFixedRate(t *testing.T) {
	for _, want := range []string{"aresample=48000", "channel_layouts=mono", "asetnsamples=n=1024", "bandpass=f=125:width_type=h:w=50,bandpass=f=125:width_type=h:w=50", "amerge=inputs=6"} {
		if !strings.Contains(analyseFilter, want) {
			t.Errorf("the analysing filter lacks %s:\n%s", want, analyseFilter)
		}
	}
}

func TestTheWholeSoundIsMeasuredInTwoChannelsAndNotAMixToOne(t *testing.T) {
	split := strings.Index(analyseFilter, "asplit=2")
	if split < 0 || strings.Contains(analyseFilter[:split], "channel_layouts") {
		t.Fatalf("the channels are changed before the whole sound is split from the bands:\n%s", analyseFilter)
	}
	whole, _, _ := strings.Cut(analyseFilter[split:], "[f]")
	if !strings.Contains(whole, "channel_layouts=stereo") || strings.Contains(whole, "mono") {
		t.Errorf("the whole sound is not measured in two channels:\n%s", whole)
	}
}

func TestAntiphaseStereoIsMeasuredAtItsLevelAndNotAsSilence(t *testing.T) {
	b, err := os.ReadFile(measuredAntiphase)
	if err != nil {
		t.Fatal(err)
	}
	blocks := parseBlocks(strings.NewReader(string(b)))
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}
	got := banded(blocks[0], audioParams{Rate: 44100, Channels: 2})
	if len(got.Channels) != 2 || len(got.Bands) != 5 {
		t.Fatalf("the block has %d channels and %d bands, want 2 and 5", len(got.Channels), len(got.Bands))
	}
	for i, c := range got.Channels {
		if math.IsInf(c.Loudness, -1) || c.Loudness < -10 {
			t.Errorf("channel %d of antiphase stereo measures %v dB", i+1, c.Loudness)
		}
	}
}

func TestAMonoTrackMeasuresItsWholeSoundAtTheLevelOfItsChannel(t *testing.T) {
	blocks := parseBlocks(strings.NewReader(string(mustRead(t, measuredBands))))
	stereo := banded(blocks[0], audioParams{Rate: 48000, Channels: 2})
	mono := banded(blocks[0], audioParams{Rate: 48000, Channels: 1})
	if d := mono.Channels[0].Loudness - stereo.Channels[0].Loudness; math.Abs(d-10*math.Log10(2)) > 1e-9 {
		t.Errorf("a track with one channel is raised by %v dB, want 3.01", d)
	}
	if r := mono.Channels[0].Difference / stereo.Channels[0].Difference; math.Abs(r-math.Sqrt2) > 1e-9 {
		t.Errorf("a track with one channel has its differences scaled by %v, want 1.414", r)
	}
}

func TestZeroCrossingsAreCountedAsAtTheTracksOwnRate(t *testing.T) {
	blocks := parseBlocks(strings.NewReader(string(mustRead(t, measuredBands))))
	at48 := banded(blocks[0], audioParams{Rate: 48000, Channels: 2})
	at96 := banded(blocks[0], audioParams{Rate: 96000, Channels: 2})
	if at96.Channels[0].Crossings != at48.Channels[0].Crossings/2 {
		t.Errorf("crossings at 96 kHz are %v, at 48 kHz %v; want half", at96.Channels[0].Crossings, at48.Channels[0].Crossings)
	}
	if at48.Bands[0] != blocks[0].Channels[2].Loudness {
		t.Errorf("the lowest band is %v, want the third channel's %v", at48.Bands[0], blocks[0].Channels[2].Loudness)
	}
}

func TestATrackMeasuredWithoutItsFormatIsAnError(t *testing.T) {
	opt, _ := analysing(t, noParamsEnv+"=1")
	blocks, err := Analyse(t.Context(), opt, streamURL())
	if err == nil || !strings.Contains(err.Error(), "format") {
		t.Errorf("got %d blocks and error %v, want an error saying the format is unknown", len(blocks), err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestABlockMeasuredWithoutBandsKeepsItsChannelsAndHasNoBands(t *testing.T) {
	blocks := parseBlocks(strings.NewReader(recorded(t)))
	b := banded(blocks[0], audioParams{})
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

func TestThePeakOfTheLastBandIsTheBlocksPunch(t *testing.T) {
	var b strings.Builder
	b.WriteString("frame:0    pts:0       pts_time:0\n")
	for c := 1; c <= analyseWhole+analyseBands; c++ {
		fmt.Fprintf(&b, "lavfi.astats.%d.RMS_level=-20.000000\n", c)
		fmt.Fprintf(&b, "lavfi.astats.%d.Peak_level=-%d.000000\n", c, c)
	}
	b.WriteString("lavfi.astats.Overall.Number_of_samples=1024.000000\n")
	blocks := parseBlocks(strings.NewReader(b.String()))
	if len(blocks) != 1 {
		t.Fatalf("one block was written and %d were read", len(blocks))
	}
	got := banded(blocks[0], audioParams{}).Punch
	// The last band's peak is -7 dB.
	if want := math.Pow(10, -7.0/20); math.Abs(got-want) > 1e-6 {
		t.Errorf("the punch was %.4f, want %.4f", got, want)
	}
	if p := banded(parseBlocks(strings.NewReader(recorded(t)))[0], audioParams{}).Punch; p != 0 {
		t.Errorf("a block measured without bands has a punch of %v", p)
	}
}
