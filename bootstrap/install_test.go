// Package bootstrap holds the installer and the tests that keep it agreeing
// with the release that produces what it downloads.
package bootstrap

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The failure this exists to prevent: a release that publishes one filename
// while the installer asks for another. Both sides are green, the tag is
// green, and the install command 404s for everyone who runs it.
func TestTheInstallerAsksForWhatTheReleaseProduces(t *testing.T) {
	goreleaser := read(t, "../.goreleaser.yaml")
	installer := read(t, "install.sh")

	name := regexp.MustCompile(`name_template:\s*"([^"]+)"`).FindStringSubmatch(goreleaser)
	if name == nil {
		t.Fatal("no archive name_template in .goreleaser.yaml; this test has stopped matching")
	}
	// The template as the shell writes it: goreleaser fills Os and Arch, the
	// installer substitutes the variables it derived from uname.
	shell := strings.NewReplacer("{{ .Os }}", "${os}", "{{ .Arch }}", "${arch}").Replace(name[1])
	want := `archive="` + shell + `.tar.gz"`

	if !strings.Contains(installer, want) {
		t.Errorf("goreleaser publishes %q, so install.sh should contain:\n  %s", name[1], want)
	}
	if !strings.Contains(goreleaser, "formats: [tar.gz]") {
		t.Error(".goreleaser.yaml no longer produces tar.gz, which install.sh untars")
	}
}

// The two files the installer fetches besides the archive, each named in a
// different place: checksums.txt by goreleaser, attestation.jsonl by the
// release workflow.
func TestTheInstallerAsksForTheRightSidecars(t *testing.T) {
	installer := read(t, "install.sh")

	sums := regexp.MustCompile(`checksum:\s*\n\s*name_template:\s*(\S+)`).
		FindStringSubmatch(read(t, "../.goreleaser.yaml"))
	if sums == nil {
		t.Fatal("no checksum name_template in .goreleaser.yaml")
	}
	if !strings.Contains(installer, sums[1]) {
		t.Errorf("goreleaser writes %q and install.sh does not fetch it", sums[1])
	}

	release := read(t, "../.github/workflows/release.yml")
	bundle := regexp.MustCompile(`(attestation\.\w+)`).FindStringSubmatch(release)
	if bundle == nil {
		t.Fatal("the release workflow publishes no attestation bundle")
	}
	if !strings.Contains(installer, bundle[1]) {
		t.Errorf("the release publishes %q and --verify does not fetch it", bundle[1])
	}
}

// A script that is piped into sh must stop at the first failure, or a failed
// download becomes an install of nothing followed by a confident success.
func TestTheInstallerStopsOnTheFirstFailure(t *testing.T) {
	installer := read(t, "install.sh")
	if !strings.Contains(installer, "\nset -eu\n") {
		t.Error("install.sh does not set -eu")
	}
	if !strings.Contains(installer, `trap 'rm -rf "$tmp"' EXIT`) {
		t.Error("install.sh does not clean up its work directory")
	}
}

// §11: the checksum proves the bytes match the release, the attestation proves
// who built them, and the installer says which it did. An installer that
// implies a check it skipped is worse than one that checks nothing.
func TestTheInstallerIsHonestAboutWhatItVerified(t *testing.T) {
	installer := read(t, "install.sh")
	for _, line := range []string{
		"checksum verified",
		"no checksum available; skipping verification",
		"provenance verified",
		"run with --verify to check provenance too",
	} {
		if !strings.Contains(installer, line) {
			t.Errorf("install.sh never says %q", line)
		}
	}
}

// shanty plays through mpv and decodes nothing itself (ADR-011). An installer
// that finishes without saying so leaves a binary that starts and cannot play.
func TestTheInstallerNamesThePrerequisite(t *testing.T) {
	installer := read(t, "install.sh")
	if !strings.Contains(installer, "command -v mpv") {
		t.Error("install.sh does not check whether mpv is present")
	}
	for _, manager := range []string{"apt install mpv", "dnf install mpv", "brew install mpv"} {
		if !strings.Contains(installer, manager) {
			t.Errorf("install.sh does not name %q", manager)
		}
	}
}

// Nothing may be fetched from a host the reader did not see named at the top.
func TestTheInstallerFetchesOnlyFromTheReleaseItNamed(t *testing.T) {
	installer := read(t, "install.sh")
	for _, url := range regexp.MustCompile(`https?://[^\s"']+`).FindAllString(installer, -1) {
		switch {
		case strings.HasPrefix(url, "https://github.com/$REPO/releases"),
			strings.HasPrefix(url, "https://raw.githubusercontent.com/bspeelm/shanty/"):
		default:
			t.Errorf("install.sh reaches %s, which is not the release it named", url)
		}
	}
}
