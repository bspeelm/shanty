package mpv

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// recordedMetadata is af-metadata/shanty as mpv 0.41 returned it while
// playing a 44.1 kHz stereo FLAC of filtered noise, trimmed to the keys read
// and a few of the others.
const recordedMetadata = `{
	"lavfi.astats.1.RMS_difference": "3654.060112",
	"lavfi.astats.1.RMS_level": "-23.418253",
	"lavfi.astats.1.Zero_crossings": "5660.000000",
	"lavfi.astats.1.Zero_crossings_rate": "0.104290",
	"lavfi.astats.2.RMS_difference": "3654.060112",
	"lavfi.astats.2.RMS_level": "-23.418253",
	"lavfi.astats.2.Zero_crossings": "5660.000000",
	"lavfi.astats.Overall.Number_of_samples": "54272.000000",
	"lavfi.astats.Overall.RMS_level": "-23.418253",
	"lavfi.astats.Overall.Noise_floor": "nan"
}`

const recordedParams = `{"samplerate": 44100, "channel-count": 2, "channels": "stereo", "format": "s16"}`

func TestAReadingOfTheAudioIsParsedFromWhatMpvRecorded(t *testing.T) {
	h, err := hearingFrom(json.RawMessage(recordedMetadata), json.RawMessage(recordedParams), json.RawMessage(`1.5`))
	if err != nil {
		t.Fatal(err)
	}
	ch := Channel{Loudness: -23.418253, Difference: 3654.060112, Crossings: 5660}
	if h.Samples != 54272 || h.Rate != 44100 || h.Scale != 32768 || h.Position != 1500*time.Millisecond {
		t.Errorf("got %+v", h)
	}
	if len(h.Channels) != 2 || h.Channels[0] != ch || h.Channels[1] != ch {
		t.Errorf("channels were read as %+v, want two of %+v", h.Channels, ch)
	}
}

func TestSilenceIsReadAsNegativeInfinity(t *testing.T) {
	meta := `{"lavfi.astats.1.RMS_level": "-inf", "lavfi.astats.1.RMS_difference": "0.000000",
		"lavfi.astats.1.Zero_crossings": "0.000000", "lavfi.astats.Overall.Number_of_samples": "2048.000000"}`
	params := `{"samplerate": 48000, "channel-count": 1, "format": "floatp"}`
	h, err := hearingFrom(json.RawMessage(meta), json.RawMessage(params), json.RawMessage(`0`))
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(h.Channels[0].Loudness, -1) {
		t.Errorf("silence was read as %v, want negative infinity", h.Channels[0].Loudness)
	}
	if h.Scale != 1 {
		t.Errorf("a float format has full scale %v, want 1", h.Scale)
	}
}

func TestAReadingWithAnythingMissingIsNoReading(t *testing.T) {
	full := map[string]string{"meta": recordedMetadata, "params": recordedParams, "pos": `1.5`}
	for name, broken := range map[string]map[string]string{
		"no metadata, as before the first block": {"meta": `null`},
		"metadata without the sample count":      {"meta": strings.Replace(recordedMetadata, "Number_of_samples", "x", 1)},
		"metadata without a channel's crossings": {"meta": strings.Replace(recordedMetadata, "2.Zero_crossings", "x", 1)},
		"no audio, as with nothing playing":      {"params": `null`},
		"no position, as with nothing playing":   {"pos": `null`},
		"more channels than were measured":       {"params": `{"samplerate": 44100, "channel-count": 6, "format": "s16"}`},
	} {
		got := map[string]string{}
		for k, v := range full {
			got[k] = v
		}
		for k, v := range broken {
			got[k] = v
		}
		_, err := hearingFrom(json.RawMessage(got["meta"]), json.RawMessage(got["params"]), json.RawMessage(got["pos"]))
		if !errors.Is(err, errNoHearing) {
			t.Errorf("%s: got %v, want errNoHearing", name, err)
		}
	}
}

func TestFullScaleFollowsTheSampleFormat(t *testing.T) {
	for format, want := range map[string]float64{
		"s16": 32768, "s16p": 32768, "s32": 1 << 31, "u8": 128, "float": 1, "floatp": 1, "double": 1,
	} {
		if got := fullScale(format); got != want {
			t.Errorf("%s: full scale %v, want %v", format, got, want)
		}
	}
}

func TestHearingAsksForTheFilterTheAudioAndThePosition(t *testing.T) {
	p, dir := stub(t)
	// The stub answers every property with null, which is no reading.
	if _, err := p.Hearing(t.Context()); !errors.Is(err, errNoHearing) {
		t.Fatalf("got %v, want errNoHearing", err)
	}
	got := commands(t, dir)
	for _, want := range []string{
		`["get_property","af-metadata/shanty"]`,
		`["get_property","audio-params"]`,
		`["get_property","time-pos"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mpv never received %s\nreceived:\n%s", want, got)
		}
	}
}

func TestMpvStartsWithTheMeasuringFilter(t *testing.T) {
	_, dir := stub(t)
	want := "--af=@shanty:lavfi=[asetnsamples=n=1024:p=0,astats=metadata=1:reset=64]"
	if argv := report(t, dir, argvFile); !strings.Contains(argv, want) {
		t.Errorf("mpv was started without the measuring filter %s:\n%s", want, argv)
	}
}
