package mpv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The measuring filter cuts the audio into blocks of blockSamples and its
// totals restart every epochBlocks blocks.
const (
	blockSamples = 1024
	epochBlocks  = 64
	// EpochSamples is how many samples per channel the totals cover before
	// they restart.
	EpochSamples = blockSamples * epochBlocks
)

// listenFilter is the audio filter mpv is started with. asetnsamples cuts the
// audio into equal blocks without padding the last one, and astats measures
// the samples that pass through it and attaches the totals to the audio as
// metadata. Neither changes the samples or their format.
var listenFilter = fmt.Sprintf("@shanty:lavfi=[asetnsamples=n=%d:p=0,astats=metadata=1:reset=%d]",
	blockSamples, epochBlocks)

// samplesKey is the metadata key for the number of samples the totals cover.
const samplesKey = "lavfi.astats.Overall.Number_of_samples"

// Hearing is one reading of the measuring filter.
type Hearing struct {
	// Samples is how many samples per channel the totals cover. It restarts
	// every EpochSamples, when a track starts and after a seek.
	Samples float64
	// Channels holds the totals for each channel.
	Channels []Channel
	// Rate is the sample rate of the audio the filter measures, and Scale is
	// the value of a full-scale sample in the units of Channel.Difference.
	Rate  int
	Scale float64
	// Position is the playback position at the moment of the reading, and At
	// is the time on the clock when mpv reported it.
	Position time.Duration
	At       time.Time
}

// Channel is the filter's totals for one channel.
type Channel struct {
	// Loudness is the RMS level in dB relative to full scale; silence is
	// negative infinity.
	Loudness float64
	// Difference is the RMS of the difference between neighbouring samples.
	Difference float64
	// Crossings is the number of zero crossings.
	Crossings float64
	// Peak is the peak level in dB relative to full scale, where the filter
	// measures it, and 0 where it does not.
	Peak float64
}

// errNoHearing is returned when mpv has no reading to give: nothing is
// playing, the first block of a track has not been decoded yet, or the player
// was started without the filter.
var errNoHearing = errors.New("mpv has no reading of the audio")

// Hearing asks mpv for the measuring filter's latest totals, the format of the
// audio and the playback position.
func (p *Player) Hearing(ctx context.Context) (Hearing, error) {
	meta, err := p.command(ctx, "get_property", "af-metadata/shanty")
	if err != nil {
		return Hearing{}, err
	}
	params, err := p.command(ctx, "get_property", "audio-params")
	if err != nil {
		return Hearing{}, err
	}
	pos, err := p.command(ctx, "get_property", "time-pos")
	if err != nil {
		return Hearing{}, err
	}
	at := time.Now()
	h, err := hearingFrom(meta, params, pos)
	h.At = at
	return h, err
}

// hearingFrom builds a Hearing from the three property values mpv returned.
// It fails when any of them is missing or is not a number.
func hearingFrom(meta, params, pos json.RawMessage) (Hearing, error) {
	var fields map[string]string
	var audio struct {
		Rate     int    `json:"samplerate"`
		Channels int    `json:"channel-count"`
		Format   string `json:"format"`
	}
	var at *float64
	if json.Unmarshal(meta, &fields) != nil ||
		json.Unmarshal(params, &audio) != nil ||
		json.Unmarshal(pos, &at) != nil ||
		audio.Rate <= 0 || audio.Channels <= 0 || at == nil {
		return Hearing{}, errNoHearing
	}
	h := Hearing{Rate: audio.Rate, Scale: fullScale(audio.Format), Position: time.Duration(*at * float64(time.Second))}
	var err error
	if h.Samples, err = strconv.ParseFloat(fields[samplesKey], 64); err != nil {
		return Hearing{}, errNoHearing
	}
	for c := 1; c <= audio.Channels; c++ {
		var ch Channel
		var errs [3]error
		prefix := "lavfi.astats." + strconv.Itoa(c) + "."
		ch.Loudness, errs[0] = strconv.ParseFloat(fields[prefix+"RMS_level"], 64)
		ch.Difference, errs[1] = strconv.ParseFloat(fields[prefix+"RMS_difference"], 64)
		ch.Crossings, errs[2] = strconv.ParseFloat(fields[prefix+"Zero_crossings"], 64)
		if errors.Join(errs[:]...) != nil {
			return Hearing{}, errNoHearing
		}
		h.Channels = append(h.Channels, ch)
	}
	return h, nil
}

// fullScale returns the value of a full-scale sample for one of mpv's sample
// format names, with or without the trailing p of a planar format.
func fullScale(format string) float64 {
	switch strings.TrimSuffix(format, "p") {
	case "u8":
		return 1 << 7
	case "s16":
		return 1 << 15
	case "s32":
		return 1 << 31
	case "s64":
		return 1 << 63
	}
	return 1
}
