package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/config"
)

// withConfig is an app whose setting is written to a scratch config file, the
// way a real one writes to config.toml.
func withConfig(t *testing.T, cfg config.Config) (app, string) {
	t.Helper()
	a, _ := withPlaylists(t, nil)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	a.configFile = path
	a.autoVolume = cfg.AutoVolume
	return a, path
}

// TestAutoVolumeReachesThePlayerAndIsRemembered covers both halves: the player
// is told, and the answer survives the session that gave it.
func TestAutoVolumeReachesThePlayerAndIsRemembered(t *testing.T) {
	a, path := withConfig(t, config.Config{Server: "https://example.org", Username: "skipper"})
	rec := a.player.(*recorder)

	a = runCommand(t, a, "auto-vol on")

	if !strings.Contains(strings.Join(rec.said(), " "), "replaygain true") {
		t.Errorf("the player was not told to level: %q", strings.Join(rec.said(), " "))
	}
	saved, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.AutoVolume {
		t.Error("the setting was not written to config.toml")
	}
	if !strings.Contains(a.ui.Status(), "levelling") {
		t.Errorf("it said %q", a.ui.Status())
	}

	// And off again, both places.
	a = runCommand(t, a, "auto-vol off")
	if !strings.Contains(strings.Join(rec.said(), " "), "replaygain false") {
		t.Errorf("the player was not told to stop: %q", strings.Join(rec.said(), " "))
	}
	saved, _ = config.LoadConfig(path)
	if saved.AutoVolume {
		t.Error("turning it off was not written")
	}
}

// TestAutoVolumeWithNoArgumentChangesIt covers the toggle. The interface does
// not hold the setting, so it asks for whichever the caller's is not.
func TestAutoVolumeWithNoArgumentChangesIt(t *testing.T) {
	a, path := withConfig(t, config.Config{
		Server: "https://example.org", Username: "skipper", AutoVolume: true})

	a = runCommand(t, a, "auto-vol")

	saved, _ := config.LoadConfig(path)
	if saved.AutoVolume {
		t.Error("an empty argument did not change the setting")
	}
	if !strings.Contains(strings.Join(a.player.(*recorder).said(), " "), "replaygain false") {
		t.Error("the player was not told")
	}
}

// TestAutoVolumeIsAppliedAtStartup covers a setting that is remembered and then
// ignored, which is the same as not remembering it.
func TestAutoVolumeIsAppliedAtStartup(t *testing.T) {
	a, _ := withConfig(t, config.Config{
		Server: "https://example.org", Username: "skipper", AutoVolume: true})

	for _, m := range runAll(a.Init()) {
		if m == nil {
			continue
		}
		a, _ = step(t, a, m)
	}

	if !strings.Contains(strings.Join(a.player.(*recorder).said(), " "), "replaygain true") {
		t.Errorf("a remembered setting was not applied at startup: %q", strings.Join(a.player.(*recorder).said(), " "))
	}
}

// TestAutoVolumeSurvivesNoConfigFile covers a session, which has nowhere to
// write and must still level.
func TestAutoVolumeSurvivesNoConfigFile(t *testing.T) {
	a, _ := withPlaylists(t, nil)
	a.configFile = ""

	a = runCommand(t, a, "auto-vol on")

	if !strings.Contains(strings.Join(a.player.(*recorder).said(), " "), "replaygain true") {
		t.Error("the player was not told when there was nowhere to save")
	}
	if strings.Contains(a.ui.Status(), "could not be saved") {
		t.Errorf("having nowhere to save was reported as a failure: %q", a.ui.Status())
	}
}

// TestAutoVolumeRefusesWhatIsNotOnOrOff covers the closed set.
func TestAutoVolumeRefusesWhatIsNotOnOrOff(t *testing.T) {
	a, _ := withConfig(t, config.Config{Server: "https://example.org", Username: "skipper"})

	a = runCommand(t, a, "auto-vol loud")

	if !strings.Contains(a.ui.Status(), "on or off") {
		t.Errorf("it said %q", a.ui.Status())
	}
}
