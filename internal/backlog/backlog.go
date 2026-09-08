// Package backlog keeps the plays a server would not accept, so that they can
// be reported when it is reachable again.
//
// The file is a line of JSON per play. A play is appended while music is
// playing, so the format has to survive a write that did not finish: an
// unreadable line is dropped and the rest of the file is kept.
package backlog

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Limit is how many plays are kept. A server unreachable for a long time
// would otherwise fill the disk with a history nobody will read, so the
// oldest are dropped first.
const Limit = 5000

// Play is one listen that has not been reported.
type Play struct {
	ID string    `json:"id"`
	At time.Time `json:"at"`
}

// Add appends a play to the file, creating it and its directory if needed.
func Add(path string, play Play) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	line, err := json.Marshal(play)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Load returns the plays waiting to be reported, oldest first.
//
// A line that cannot be read is skipped rather than failing the whole file. A
// process killed mid-append leaves a partial last line, and the plays before
// it are still worth having.
func Load(path string) ([]Play, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []Play
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 4<<10), 64<<10)
	for scan.Scan() {
		var play Play
		if err := json.Unmarshal(scan.Bytes(), &play); err != nil || play.ID == "" {
			continue
		}
		out = append(out, play)
	}
	if err := scan.Err(); err != nil {
		// A file with an unreadably long line still has usable plays in it.
		return out, nil
	}
	return trim(out), nil
}

// Replace writes the given plays over whatever the file held. An empty list
// removes the file, so that a machine that has caught up leaves nothing
// behind.
func Replace(path string, plays []Play) error {
	plays = trim(plays)
	if len(plays) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	var body []byte
	for _, play := range plays {
		line, err := json.Marshal(play)
		if err != nil {
			return err
		}
		body = append(body, append(line, '\n')...)
	}

	// Written beside the file and moved over it, so that a process stopped
	// part way through leaves the old file rather than half of a new one.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// trim keeps the newest Limit plays.
func trim(plays []Play) []Play {
	if len(plays) <= Limit {
		return plays
	}
	return plays[len(plays)-Limit:]
}
