package mpv

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

// listenFilter is the audio filter mpv is started with. astats measures the
// samples that pass through it and attaches the totals to the audio as
// metadata. It does not change the samples or their format.
const listenFilter = "@shanty:lavfi=[astats=metadata=1]"

// The metadata keys Hearing reads. Both are running totals that start again
// when a track starts and after a seek.
const (
	samplesKey  = "lavfi.astats.Overall.Number_of_samples"
	loudnessKey = "lavfi.astats.Overall.RMS_level"
)

// Hearing is one reading of the measuring filter.
type Hearing struct {
	// Samples is how many samples per channel the filter has measured since
	// the track started or was last seeked.
	Samples float64
	// Loudness is the RMS level of all of those samples, in dB relative to
	// full scale. Silence is negative infinity.
	Loudness float64
	// Rate is the sample rate of the audio the filter measures.
	Rate int
	// Position is the playback position at the moment of the reading.
	Position time.Duration
}

// errNoHearing is returned when mpv has no reading to give: nothing is
// playing, the first block of a track has not been decoded yet, or the player
// was started without the filter.
var errNoHearing = errors.New("mpv has no reading of the audio")

// Hearing asks mpv for the measuring filter's latest totals, the sample rate
// and the playback position.
func (p *Player) Hearing(ctx context.Context) (Hearing, error) {
	meta, err := p.command(ctx, "get_property", "af-metadata/shanty")
	if err != nil {
		return Hearing{}, err
	}
	rate, err := p.command(ctx, "get_property", "audio-params/samplerate")
	if err != nil {
		return Hearing{}, err
	}
	pos, err := p.command(ctx, "get_property", "time-pos")
	if err != nil {
		return Hearing{}, err
	}
	return hearingFrom(meta, rate, pos)
}

// hearingFrom builds a Hearing from the three property values mpv returned.
// It fails when any of them is missing or is not a number.
func hearingFrom(meta, rate, pos json.RawMessage) (Hearing, error) {
	var fields map[string]string
	var h Hearing
	var at *float64
	if json.Unmarshal(meta, &fields) != nil ||
		json.Unmarshal(rate, &h.Rate) != nil ||
		json.Unmarshal(pos, &at) != nil ||
		h.Rate <= 0 || at == nil {
		return Hearing{}, errNoHearing
	}
	samples, err1 := strconv.ParseFloat(fields[samplesKey], 64)
	loudness, err2 := strconv.ParseFloat(fields[loudnessKey], 64)
	if err1 != nil || err2 != nil {
		return Hearing{}, errNoHearing
	}
	h.Samples, h.Loudness = samples, loudness
	h.Position = time.Duration(*at * float64(time.Second))
	return h, nil
}
