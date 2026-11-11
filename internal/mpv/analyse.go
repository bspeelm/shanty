package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// analyseFilter cuts the audio into blocks of blockSamples, measures each
// block on its own, and prints every block's totals to standard output.
var analyseFilter = fmt.Sprintf("lavfi=[asetnsamples=n=%d:p=0,"+
	"astats=metadata=1:reset=1:measure_perchannel=RMS_level+RMS_difference+Zero_crossings:measure_overall=Number_of_samples,"+
	"ametadata=mode=print:file=-]", blockSamples)

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
	binary := opt.Binary
	if binary == "" {
		binary = "mpv"
	}
	if opt.Socket == "" {
		return nil, errors.New("no IPC socket path was given")
	}
	socket := opt.Socket + analyseSuffix
	if err := checkSocketPath(socket); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return nil, err
	}
	if err := clearSocket(socket); err != nil {
		return nil, err
	}

	cmd := exec.Command(binary,
		"--no-config",
		"--no-video",
		"--idle=yes",
		"--ao=null",
		"--ao-null-untimed=yes",
		"--term-status-msg=",
		"--msg-level=all=no",
		"--af="+analyseFilter,
		"--input-ipc-server="+socket,
	)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, in := io.Pipe()
	cmd.Stdout, cmd.Stderr = in, opt.Diagnostics
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start %s: %w", binary, err)
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
		return nil, err
	}
	p := newPlayer(cmd, conn, socket)
	go p.read()
	params, err := p.follow(ctx, url)
	_ = p.Close() // Close kills an mpv that will not quit, and reports nothing
	<-exited
	_ = os.Remove(socket) // a socket left behind is cleared on the next start
	if err != nil {
		return nil, err
	}
	blocks := <-parsed
	scale := fullScale(params.Format)
	for i := range blocks {
		blocks[i].Rate, blocks[i].Scale = params.Rate, scale
	}
	return blocks, nil
}

// audioParams is the part of mpv's audio-params property the analysis uses.
type audioParams struct {
	Rate   int    `json:"samplerate"`
	Format string `json:"format"`
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
