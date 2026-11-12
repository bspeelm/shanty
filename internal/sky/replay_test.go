//go:build replay

package sky

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"testing"
	"time"
)

// recorded is one line of a recording of live readings: the clock time, the
// playback position and the measuring filter's metadata, or the start of a
// track with its title and audio parameters.
type recorded struct {
	At     float64           `json:"at"`
	Pos    *float64          `json:"pos"`
	MD     map[string]string `json:"md"`
	Title  *string           `json:"title"`
	Paused bool              `json:"paused"`
	Params *struct {
		Rate   int    `json:"samplerate"`
		Format string `json:"format"`
	} `json:"params"`
}

// recordedTrack is the readings of one track in a recording.
type recordedTrack struct {
	title    string
	readings []Reading
}

// recordedEpoch is the number of samples after which the measuring filter
// restarts its totals.
const recordedEpoch = 1024 * 64

// readRecording splits a recording into tracks of readings. A line whose
// title is "(address withheld)" is not the start of a track.
func readRecording(t *testing.T, path string) []recordedTrack {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// The file is only read, so a failure to close it loses nothing.
		_ = f.Close()
	}()
	var out []recordedTrack
	rate, scale := 48000, float64(1<<15)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r recorded
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		if r.Params != nil {
			rate = r.Params.Rate
			scale = map[string]float64{"s16": 1 << 15, "s32": 1 << 31}[r.Params.Format]
		}
		if r.Title != nil && *r.Title != "(address withheld)" {
			out = append(out, recordedTrack{title: *r.Title})
		}
		if len(out) == 0 || r.MD == nil || r.Pos == nil || r.Paused {
			continue
		}
		n, err := strconv.ParseFloat(r.MD["lavfi.astats.Overall.Number_of_samples"], 64)
		if err != nil {
			continue
		}
		rd := Reading{Samples: n, Rate: rate, Scale: scale, Epoch: recordedEpoch,
			Position: time.Duration(*r.Pos * float64(time.Second)),
			At:       time.Unix(0, int64(r.At*1e9))}
		for c := 1; c <= 2; c++ {
			p := "lavfi.astats." + strconv.Itoa(c) + "."
			ch := Channel{Loudness: math.Inf(-1)}
			// A field that does not parse, such as "-inf", leaves the value
			// as silence or zero.
			if v, err := strconv.ParseFloat(r.MD[p+"RMS_level"], 64); err == nil {
				ch.Loudness = v
			}
			ch.Difference, _ = strconv.ParseFloat(r.MD[p+"RMS_difference"], 64)
			ch.Crossings, _ = strconv.ParseFloat(r.MD[p+"Zero_crossings"], 64)
			rd.Channels = append(rd.Channels, ch)
		}
		tr := &out[len(out)-1]
		tr.readings = append(tr.readings, rd)
	}
	return out
}

// replayWindow is what is counted over ten seconds of a replay.
type replayWindow struct {
	kicks, snaps, locked, frames, jumps int
	// x and y add up the phase at each kick as a point on a circle.
	x, y float64
}

// TestReplayARecording plays a recording named by SKY_READINGS through a
// listener at thirty frames a second of the recorded clock, and prints for
// every ten seconds of each track: the kicks and snaps heard, the tempo and
// its confidence at the end, the share of frames with a tempo, the mean
// phase at the kicks and how closely the kicks gather around it, and the
// frames on which the phase moved more than 0.08 of a beat from where the
// tempo would take it. With SKY_ESTIMATES set it also prints every estimate
// of the tempo.
func TestReplayARecording(t *testing.T) {
	path := os.Getenv("SKY_READINGS")
	if path == "" {
		t.Skip("SKY_READINGS is not set")
	}
	estimates := os.Getenv("SKY_ESTIMATES") != ""
	dt := 1.0 / 30
	for _, tr := range readRecording(t, path) {
		if len(tr.readings) < 300 {
			continue
		}
		start, end := tr.readings[0].At, tr.readings[len(tr.readings)-1].At
		fmt.Printf("== %s, %.0f s\n", tr.title, end.Sub(start).Seconds())
		var l listener
		var w replayWindow
		var prev Signals
		next, until := 0, 10.0
		for at := 0.0; ; at += dt {
			now := start.Add(time.Duration(at * float64(time.Second)))
			if now.After(end) {
				break
			}
			for next < len(tr.readings) && !tr.readings[next].At.After(now) {
				l.ear.hear(tr.readings[next])
				next++
			}
			steps := l.tempo.steps
			s := l.step(dt, now)
			if estimates && l.tempo.steps/15 != steps/15 && len(l.tempo.onsets) >= tempoWindow/2 {
				fmt.Printf("    %7.2f s  estimate %6.1f  strength %.2f  confidence %.2f\n", l.ear.now, l.tempo.bpm, l.tempo.sure, l.tempo.conf)
			}
			w.frames++
			if s.Kick > 0.5 && prev.Kick <= 0.5 {
				w.kicks++
				if s.Tempo > 0 {
					sn, cs := math.Sincos(2 * math.Pi * s.Phase)
					w.x, w.y = w.x+cs, w.y+sn
				}
			}
			if s.Snap > 0.5 && prev.Snap <= 0.5 {
				w.snaps++
			}
			if s.Tempo > 0 {
				w.locked++
				moved := s.Phase - prev.Phase + float64(s.Beat-prev.Beat)
				if prev.Tempo > 0 && math.Abs(moved-dt*s.Tempo/60) > 0.08 {
					w.jumps++
				}
			}
			prev = s
			if at >= until {
				gather := 0.0
				if w.kicks > 0 {
					gather = math.Hypot(w.x, w.y) / float64(w.kicks)
				}
				fmt.Printf("  %5.0f s  kicks %3d  snaps %3d  tempo %6.1f  sure %.2f  held %3.0f%%  phase at kicks %+.2f (gathered %.2f)  jumps %d\n",
					l.ear.now, w.kicks, w.snaps, s.Tempo, s.Sure, 100*float64(w.locked)/float64(w.frames),
					math.Atan2(w.y, w.x)/(2*math.Pi), gather, w.jumps)
				w = replayWindow{}
				until += 10
			}
		}
	}
}
