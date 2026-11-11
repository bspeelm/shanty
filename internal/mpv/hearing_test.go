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
// playing a 44.1 kHz stereo FLAC, trimmed to a few of its seventy keys.
const recordedMetadata = `{
	"lavfi.astats.1.RMS_level": "-23.101990",
	"lavfi.astats.Overall.DC_offset": "0.000001",
	"lavfi.astats.Overall.Peak_level": "-1.938200",
	"lavfi.astats.Overall.RMS_level": "-22.874511",
	"lavfi.astats.Overall.Number_of_samples": "77824.000000",
	"lavfi.astats.Overall.Noise_floor": "nan"
}`

func TestAReadingOfTheAudioIsParsedFromWhatMpvRecorded(t *testing.T) {
	h, err := hearingFrom(json.RawMessage(recordedMetadata), json.RawMessage(`44100`), json.RawMessage(`1.5`))
	if err != nil {
		t.Fatal(err)
	}
	want := Hearing{Samples: 77824, Loudness: -22.874511, Rate: 44100, Position: 1500 * time.Millisecond}
	if h != want {
		t.Errorf("got %+v, want %+v", h, want)
	}
}

func TestSilenceIsReadAsNegativeInfinity(t *testing.T) {
	meta := `{"lavfi.astats.Overall.RMS_level": "-inf", "lavfi.astats.Overall.Number_of_samples": "2048.000000"}`
	h, err := hearingFrom(json.RawMessage(meta), json.RawMessage(`48000`), json.RawMessage(`0`))
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(h.Loudness, -1) {
		t.Errorf("silence was read as %v, want negative infinity", h.Loudness)
	}
}

func TestAReadingWithAnythingMissingIsNoReading(t *testing.T) {
	full := map[string]string{"meta": recordedMetadata, "rate": `44100`, "pos": `1.5`}
	for name, broken := range map[string]map[string]string{
		"no metadata, as before the first block":   {"meta": `null`},
		"metadata without the sample count":        {"meta": `{"lavfi.astats.Overall.RMS_level": "-20.0"}`},
		"metadata without the loudness":            {"meta": `{"lavfi.astats.Overall.Number_of_samples": "1024"}`},
		"no sample rate, as with nothing playing":  {"rate": `null`},
		"no position, as with nothing playing":     {"pos": `null`},
		"a sample rate that is not a whole number": {"rate": `"fast"`},
	} {
		got := map[string]string{}
		for k, v := range full {
			got[k] = v
		}
		for k, v := range broken {
			got[k] = v
		}
		_, err := hearingFrom(json.RawMessage(got["meta"]), json.RawMessage(got["rate"]), json.RawMessage(got["pos"]))
		if !errors.Is(err, errNoHearing) {
			t.Errorf("%s: got %v, want errNoHearing", name, err)
		}
	}
}

func TestHearingAsksForTheFilterTheRateAndThePosition(t *testing.T) {
	p, dir := stub(t)
	// The stub answers every property with null, which is no reading.
	if _, err := p.Hearing(t.Context()); !errors.Is(err, errNoHearing) {
		t.Fatalf("got %v, want errNoHearing", err)
	}
	got := commands(t, dir)
	for _, want := range []string{
		`["get_property","af-metadata/shanty"]`,
		`["get_property","audio-params/samplerate"]`,
		`["get_property","time-pos"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mpv never received %s\nreceived:\n%s", want, got)
		}
	}
}

func TestMpvStartsWithTheMeasuringFilter(t *testing.T) {
	_, dir := stub(t)
	if argv := report(t, dir, argvFile); !strings.Contains(argv, "--af="+listenFilter) {
		t.Errorf("mpv was started without the measuring filter:\n%s", argv)
	}
}
