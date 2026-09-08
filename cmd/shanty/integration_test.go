//go:build integration

// The one test that talks to a real server and a real player.
//
// Everything else in this repository runs against the fake, which is fast,
// offline and honest about being a fake. What it cannot tell us is whether
// mpv accepts the flags §7 gives it, and whether Navidrome's answers have the
// shape internal/subsonic decodes. Both are unverified until this runs.
//
// Build-tagged and excluded from `make check` on purpose: it needs a container
// runtime, a network to pull an image, and mpv installed. `make check` must
// stay offline (§10).
//
//	go test -tags=integration ./cmd/shanty/ -v -timeout=10m
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/subsonic"
)

const (
	navidromeImage = "deluan/navidrome:latest"
	adminUser      = "admin"
	adminPassword  = "integration-only-not-a-secret"
	scanTimeout    = 3 * time.Minute
)

// runtimeCLI finds docker or podman. This is a machine question, which every
// other test in this repository is forbidden from asking -- and it is why this
// one carries a build tag.
func runtimeCLI(t *testing.T) string {
	t.Helper()
	for _, cli := range []string{"docker", "podman"} {
		if path, err := exec.LookPath(cli); err == nil {
			return path
		}
	}
	t.Skip("neither docker nor podman is installed")
	return ""
}

func requireMpv(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv is not installed; this is the test that needs it")
	}
	return path
}

// silence writes a WAV of the given length with RIFF INFO tags, which is what
// Navidrome reads to build a library. Generated rather than committed: a
// binary fixture in the tree is one nobody can read the diff of.
func silence(t *testing.T, path string, seconds int, artist, album, title string, track int) {
	t.Helper()
	const rate = 8000
	samples := rate * seconds
	data := make([]byte, samples*2)

	info := riffInfo(map[string]string{
		"IART": artist, "IPRD": album, "INAM": title, "ITRK": fmt.Sprint(track),
	})

	var b []byte
	b = append(b, "RIFF"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(4+24+8+len(data)+len(info)))
	b = append(b, "WAVE"...)
	b = append(b, "fmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1)      // PCM
	b = binary.LittleEndian.AppendUint16(b, 1)      // mono
	b = binary.LittleEndian.AppendUint32(b, rate)   // sample rate
	b = binary.LittleEndian.AppendUint32(b, rate*2) // byte rate
	b = binary.LittleEndian.AppendUint16(b, 2)      // block align
	b = binary.LittleEndian.AppendUint16(b, 16)     // bits
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
	b = append(b, data...)
	b = append(b, info...)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func riffInfo(tags map[string]string) []byte {
	var body []byte
	body = append(body, "INFO"...)
	for id, value := range tags {
		value += "\x00"
		if len(value)%2 == 1 {
			value += "\x00"
		}
		body = append(body, id...)
		body = binary.LittleEndian.AppendUint32(body, uint32(len(value)))
		body = append(body, value...)
	}
	var out []byte
	out = append(out, "LIST"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(body)))
	return append(out, body...)
}

// navidrome starts a real server against a generated library and returns its
// URL. The container is removed however the test ends.
func navidrome(t *testing.T) string {
	t.Helper()
	cli := runtimeCLI(t)

	music := t.TempDir()
	silence(t, filepath.Join(music, "Aoi", "Harbour", "01 Slipway.wav"), 2, "Aoi", "Harbour", "Slipway", 1)
	silence(t, filepath.Join(music, "Aoi", "Harbour", "02 Ballast.wav"), 2, "Aoi", "Harbour", "Ballast", 2)
	// World-readable, because the container runs as another user.
	_ = filepath.WalkDir(music, func(p string, _ os.DirEntry, _ error) error {
		return os.Chmod(p, 0o777)
	})

	name := "shanty-integration-" + fmt.Sprint(time.Now().UnixNano())
	args := []string{
		"run", "--rm", "-d", "--name", name,
		"-p", "0:4533",
		"-v", music + ":/music:ro",
		"-e", "ND_MUSICFOLDER=/music",
		"-e", "ND_LOGLEVEL=error",
		"-e", "ND_ENABLEINSIGHTS=false",
		"-e", "ND_SCANSCHEDULE=0",
		"-e", "ND_DEVAUTOCREATEADMINPASSWORD=" + adminPassword,
		navidromeImage,
	}
	if out, err := exec.Command(cli, args...).CombinedOutput(); err != nil {
		t.Fatalf("starting navidrome: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		if out, err := exec.Command(cli, "rm", "-f", name).CombinedOutput(); err != nil {
			t.Logf("removing the container: %v\n%s", err, out)
		}
	})

	port, err := exec.Command(cli, "port", name, "4533/tcp").Output()
	if err != nil {
		t.Fatalf("finding the mapped port: %v", err)
	}
	_, mapped, found := strings.Cut(strings.TrimSpace(strings.Split(string(port), "\n")[0]), ":")
	if !found {
		t.Fatalf("could not read a port out of %q", port)
	}
	return "http://127.0.0.1:" + mapped
}

// The vertical slice §10 asks for: auth, browse, stream a track to completion
// through real mpv, scrobble. Subtests are named so the CI job can assert each
// one ran rather than trusting a green tick over a suite that skipped.
func TestIntegrationVerticalSlice(t *testing.T) {
	mpvPath := requireMpv(t)
	server := navidrome(t)

	client, err := subsonic.New(server, subsonic.PasswordAuth(adminUser, adminPassword), subsonic.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()

	t.Run("auth", func(t *testing.T) {
		waitFor(t, ctx, 2*time.Minute, "the server to answer", func() bool {
			return client.Ping(ctx) == nil
		})
	})

	var album subsonic.Album
	t.Run("browse", func(t *testing.T) {
		var artists []subsonic.Artist
		waitFor(t, ctx, scanTimeout, "the library to be scanned", func() bool {
			var err error
			artists, err = client.Artists(ctx)
			return err == nil && len(artists) > 0
		})
		t.Logf("navidrome reported %d artists; first is %q", len(artists), artists[0].Name)

		artist, err := client.Artist(ctx, artists[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(artist.Albums) == 0 {
			t.Fatalf("artist %q has no albums", artist.Name)
		}
		if album, err = client.Album(ctx, artist.Albums[0].ID); err != nil {
			t.Fatal(err)
		}
		if len(album.Songs) == 0 {
			t.Fatalf("album %q has no songs", album.Name)
		}
		// The fidelity question the fake cannot answer: does what Navidrome
		// sends decode into the shapes this client expects?
		for _, s := range album.Songs {
			if s.ID == "" || s.Title == "" {
				t.Errorf("a song decoded with an empty id or title: %+v", s)
			}
		}
	})

	t.Run("stream", func(t *testing.T) {
		socket := filepath.Join(t.TempDir(), "mpv.sock")
		var said bytes.Buffer
		player, err := mpv.Start(ctx, mpv.Options{Binary: mpvPath, Socket: socket, Stderr: &said})
		if err != nil {
			t.Fatalf("starting real mpv: %v", err)
		}
		defer player.Close()

		// This is what §7 has never been able to prove: real mpv, given the
		// real flags, accepting a real credential-bearing URL over IPC.
		if err := player.Load(ctx, client.StreamURL(album.Songs[0].ID)); err != nil {
			t.Fatalf("mpv refused the track: %v", err)
		}

		deadline := time.After(90 * time.Second)
		for {
			select {
			case e, open := <-player.Events():
				if !open {
					t.Fatalf("mpv stopped while playing: %v", player.Err())
				}
				if e.Name == "end-file" {
					if e.Reason != "eof" {
						t.Fatalf("the track ended with reason %q, not eof.\nmpv said:\n%s", e.Reason, said.String())
					}
					return
				}
			case <-deadline:
				t.Fatal("a two-second track did not finish in ninety seconds")
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	})

	t.Run("scrobble", func(t *testing.T) {
		if err := client.Scrobble(ctx, album.Songs[0].ID, true); err != nil {
			t.Fatalf("the server refused a scrobble: %v", err)
		}
	})
}

func waitFor(t *testing.T, ctx context.Context, limit time.Duration, what string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Fatalf("waited %s for %s", limit, what)
}
