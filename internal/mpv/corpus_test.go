//go:build corpus

package mpv

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wholeFilter is the analysing filter as it was before the bands: the whole
// sound in the track's own channels at its own rate.
var wholeFilter = fmt.Sprintf("lavfi=[asetnsamples=n=%d:p=0,"+
	"astats=metadata=1:reset=1:measure_perchannel=RMS_level+RMS_difference+Zero_crossings:measure_overall=Number_of_samples,"+
	"ametadata=mode=print:file=-]", blockSamples)

// TestCorpusWholeSound measures every audio file in SHANTY_CORPUS with
// wholeFilter and writes its blocks to SHANTY_CORPUS_WHOLE/<name>.gob, for
// the corpus evaluation to compare the cues of the two analyses. A file
// already written is kept. It runs with
//
//	go test -tags=corpus -run CorpusWholeSound ./internal/mpv
func TestCorpusWholeSound(t *testing.T) {
	dir, out := os.Getenv("SHANTY_CORPUS"), os.Getenv("SHANTY_CORPUS_WHOLE")
	if dir == "" || out == "" {
		t.Skip("SHANTY_CORPUS and SHANTY_CORPUS_WHOLE are not set")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.flac"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		path := filepath.Join(out, name+".gob")
		if _, err := os.Stat(path); err == nil {
			continue
		}
		sock, err := os.MkdirTemp("/tmp", "sh")
		if err != nil {
			t.Fatal(err)
		}
		blocks, params, err := analyse(t.Context(), Options{Socket: filepath.Join(sock, "mpv.sock")}, file, wholeFilter)
		_ = os.RemoveAll(sock) // a temporary directory left behind is harmless
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for i := range blocks {
			blocks[i].Rate, blocks[i].Scale = params.Rate, fullScale(params.Format)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := gob.NewEncoder(f).Encode(blocks); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d blocks at %d Hz", name, len(blocks), params.Rate)
	}
}
