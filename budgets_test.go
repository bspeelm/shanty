package shanty

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The panic count the budget script reports for this project's own code.
var reportedPanics = regexp.MustCompile(`(?m)^panics:\s+(\d+) \(budget`)

// linesMatching counts the lines of every Go file under dir that contain want.
func linesMatching(dir, want string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, want) {
				n++
			}
		}
		return nil
	})
	return n
}

// budgetReport runs the budget script and returns what it printed. A failing
// budget is not an error here: an unrelated count going over must not decide
// what this file asserts, and the script prints every line either way.
func budgetReport(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("sh", "scripts/budgets.sh").CombinedOutput()
	if len(out) == 0 {
		t.Fatalf("the budget script printed nothing: %v", err)
	}
	return string(out)
}

// A vendored dependency contains panics and the budget is zero, so the two
// agree only while the script is excluding vendor/ on the platform it runs
// on. Counted here rather than assumed, because a script that excluded
// nothing would report the same zero against an empty vendor directory.
func TestTheBudgetScriptDoesNotCountVendoredPanics(t *testing.T) {
	if _, err := os.Stat("vendor"); err != nil {
		t.Skip("no vendor/ directory, so excluding it proves nothing")
	}
	vendored := linesMatching("vendor", "panic(")
	if vendored == 0 {
		t.Fatal("vendor/ holds no panic( at all, so this test proves nothing; " +
			"assert against a dependency that has one")
	}
	t.Logf("vendor/ holds %d lines with panic(", vendored)

	m := reportedPanics.FindStringSubmatch(budgetReport(t))
	if m == nil {
		t.Fatal("the budget script reported no panic count; the report has changed shape")
	}
	if m[1] != "0" {
		t.Errorf("the budget script counts %s panics; this project has %d and vendor/ has %d",
			m[1], linesMatching("cmd", "panic(")+linesMatching("internal", "panic("), vendored)
	}
}
