package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// analyseRate is the sample rate the analysing filter measures at,
// analyseWhole is how many channels it measures the whole sound in, and
// analyseBands is how many bands it measures besides.
const (
	analyseRate  = 48000
	analyseWhole = 2
	analyseBands = 4
)

// analyseFilter resamples the audio to analyseRate, cuts it into blocks of
// blockSamples, and measures each block on its own: the whole sound in two
// channels, then four bands of a mix to one channel, which are below 150 Hz,
// 100 to 400 Hz, 1 to 4 kHz and above 6 kHz. It prints every block's totals
// to standard output.
var analyseFilter = fmt.Sprintf("lavfi=[asplit=2[g][b];"+
	"[g]aresample=%[1]d,aformat=sample_fmts=flt:channel_layouts=stereo,asetnsamples=n=%[2]d:p=0[f];"+
	"[b]aresample=%[1]d,aformat=sample_fmts=flt:channel_layouts=mono,asetnsamples=n=%[2]d:p=0,asplit=4[w][x][y][z];"+
	"[w]lowpass=f=150,aformat=channel_layouts=mono[k];"+
	"[x]bandpass=f=250:width_type=h:w=300,aformat=channel_layouts=mono[l];"+
	"[y]bandpass=f=2500:width_type=h:w=3000,aformat=channel_layouts=mono[m];"+
	"[z]highpass=f=6000,aformat=channel_layouts=mono[h];"+
	"[f][k][l][m][h]amerge=inputs=5,"+
	"astats=metadata=1:reset=1:measure_perchannel=RMS_level+RMS_difference+Zero_crossings:measure_overall=Number_of_samples,"+
	"ametadata=mode=print:file=-]", analyseRate, blockSamples)

// analyseSuffix is added to the player's socket path to name the socket of
// the analysing mpv.
const analyseSuffix = ".ahead"

// Block is the measuring filter's totals for one block of a track.
type Block struct {
	// At is where the block starts in the track.
	At time.Duration
	// Samples is how many samples per channel the block holds.
	Samples float64
	// Channels holds the totals for each channel.
	Channels []Channel
	// Bands holds the RMS level in dB relative to full scale of each band the
	// block was measured in, lowest first. It is empty for a block measured
	// without bands.
	Bands []float64
	// Rate is the sample rate, and Scale is the value of a full-scale sample
	// in the units of Channel.Difference.
	Rate  int
	Scale float64
}

// Analyse measures a whole track ahead of playback. It starts a second mpv
// that plays nothing aloud and decodes as fast as it can, loads the URL over
// that mpv's own socket, and returns the totals of every block. The socket is
// opt.Socket with a suffix. Cancelling ctx stops the mpv and returns
// ctx.Err().
func Analyse(ctx context.Context, opt Options, url string) ([]Block, error) {
	blocks, params, err := analyse(ctx, opt, url, analyseFilter)
	if err != nil {
		return nil, err
	}
	for i := range blocks {
		blocks[i] = banded(blocks[i], params)
	}
	return blocks, nil
}

// analyse measures a track with a filter, and returns the totals of every
// block and the format of the decoded audio.
func analyse(ctx context.Context, opt Options, url, filter string) ([]Block, audioParams, error) {
	var none audioParams
	binary := opt.Binary
	if binary == "" {
		binary = "mpv"
	}
	if opt.Socket == "" {
		return nil, none, errors.New("no IPC socket path was given")
	}
	socket := opt.Socket + analyseSuffix
	if err := checkSocketPath(socket); err != nil {
		return nil, none, err
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return nil, none, err
	}
	if err := clearSocket(socket); err != nil {
		return nil, none, err
	}

	cmd := exec.Command(binary,
		"--no-config",
		"--no-video",
		"--idle=yes",
		"--ao=null",
		"--ao-null-untimed=yes",
		"--term-status-msg=",
		"--msg-level=all=no",
		"--af="+filter,
		"--input-ipc-server="+socket,
	)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, in := io.Pipe()
	cmd.Stdout, cmd.Stderr = in, opt.Diagnostics
	if err := cmd.Start(); err != nil {
		return nil, none, fmt.Errorf("could not start %s: %w", binary, err)
	}
	parsed := make(chan []Block, 1)
	go func() { parsed <- parseBlocks(out) }()
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait() // how the analysing mpv ended is told by its events
		_ = in.Close() // closing a pipe writer cannot fail
		close(exited)
	}()

	conn, err := dial(ctx, socket)
	if err != nil {
		_ = cmd.Process.Kill() // the process may have exited already
		<-exited
		return nil, none, err
	}
	p := newPlayer(cmd, conn, socket)
	go p.read()
	params, err := p.follow(ctx, url)
	_ = p.Close() // Close kills an mpv that will not quit, and reports nothing
	<-exited
	_ = os.Remove(socket) // a socket left behind is cleared on the next start
	if err != nil {
		return nil, none, err
	}
	return <-parsed, params, nil
}

// banded returns a block measured by analyseFilter with its rate and scale
// set. A block with the channels of the whole sound and one for each band
// keeps the first as its channels and the rest as its bands; any other block
// keeps all its channels and has no bands. The whole sound of a track with
// one channel is raised to the level of that channel, and the zero crossings
// are scaled to what they are at the track's own sample rate.
func banded(b Block, params audioParams) Block {
	b.Rate, b.Scale = analyseRate, 1
	if len(b.Channels) != analyseWhole+analyseBands {
		return b
	}
	b.Bands = make([]float64, analyseBands)
	for i, c := range b.Channels[analyseWhole:] {
		b.Bands[i] = c.Loudness
	}
	b.Channels = slices.Clone(b.Channels[:analyseWhole])
	for i := range b.Channels {
		c := &b.Channels[i]
		if params.Channels == 1 {
			c.Loudness += 10 * math.Log10(analyseWhole)
			c.Difference *= math.Sqrt(analyseWhole)
		}
		if params.Rate > 0 {
			c.Crossings *= float64(analyseRate) / float64(params.Rate)
		}
	}
	return b
}

// audioParams is the part of mpv's audio-params property the analysis uses.
type audioParams struct {
	Rate     int    `json:"samplerate"`
	Format   string `json:"format"`
	Channels int    `json:"channel-count"`
}

// follow loads a track into an analysing mpv and waits for it to end. It
// returns the format of the decoded audio.
func (p *Player) follow(ctx context.Context, url string) (audioParams, error) {
	var params audioParams
	if err := p.Observe(ctx, "audio-params"); err != nil {
		return params, err
	}
	if _, err := p.command(ctx, "loadfile", url); err != nil {
		return params, err
	}
	for {
		select {
		case <-ctx.Done():
			return params, ctx.Err()
		case e, open := <-p.events:
			if !open {
				return params, fmt.Errorf("mpv stopped before the track was measured: %w", p.Err())
			}
			switch {
			case e.Name == "property-change" && e.Property == "audio-params":
				var got audioParams
				if json.Unmarshal(e.Data, &got) == nil && got.Rate > 0 {
					params = got
				}
			case e.Name == "end-file" && e.Reason != "eof":
				return params, fmt.Errorf("mpv could not measure the track: %s", e.Reason)
			case e.Name == "end-file" && params.Rate == 0:
				return params, errors.New("mpv measured the track without saying its format")
			case e.Name == "end-file":
				return params, nil
			}
		}
	}
}

// parseBlocks reads what the analysing filter prints. Each block is a line
// naming the frame and its time, such as "frame:21 pts:21504
// pts_time:0.487619", followed by one key=value line for each total. A line
// that is neither is skipped, and so is a block with no samples or no
// channels.
func parseBlocks(r io.Reader) []Block {
	var out []Block
	var cur *Block
	keep := func() {
		if cur != nil && cur.Samples > 0 && len(cur.Channels) > 0 {
			out = append(out, *cur)
		}
		cur = nil
	}
	scan := bufio.NewScanner(r)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(line, "frame:") {
			keep()
			for _, field := range strings.Fields(line) {
				if v, found := strings.CutPrefix(field, "pts_time:"); found {
					if at, err := strconv.ParseFloat(v, 64); err == nil {
						cur = &Block{At: time.Duration(at * float64(time.Second))}
					}
				}
			}
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if cur == nil || !found {
			continue
		}
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		if key == samplesKey {
			cur.Samples = v
			continue
		}
		rest, found := strings.CutPrefix(key, "lavfi.astats.")
		number, name, _ := strings.Cut(rest, ".")
		c, err := strconv.Atoi(number)
		if !found || err != nil || c < 1 || c > 64 {
			continue
		}
		for len(cur.Channels) < c {
			cur.Channels = append(cur.Channels, Channel{})
		}
		switch ch := &cur.Channels[c-1]; name {
		case "RMS_level":
			ch.Loudness = v
		case "RMS_difference":
			ch.Difference = v
		case "Zero_crossings":
			ch.Crossings = v
		}
	}
	keep()
	return out
}
