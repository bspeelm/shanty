package shanty

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
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

// A working copy kept in a hidden directory inside the checkout holds a
// second copy of everything, vendor/ included. The budgets describe this
// checkout, so none of that copy is counted.
func TestTheBudgetScriptDoesNotCountAHiddenWorkingCopy(t *testing.T) {
	before := budgetReport(t)
	dir := ".budgets-test-copy"
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o700); err != nil {
		if errors.Is(err, syscall.EROFS) || errors.Is(err, fs.ErrPermission) {
			t.Skip("the checkout cannot be written to, so no copy can be planted in it")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) }) // a leftover is removed on the next run
	planted := "package internal\n\nfunc f() { panic(\"x\") }\n"
	if err := os.WriteFile(filepath.Join(dir, "internal", "copy.go"), []byte(planted), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte(strings.Repeat("line\n", 50)), 0o600); err != nil {
		t.Fatal(err)
	}
	after := budgetReport(t)
	for _, line := range []string{"code(raw):", "tests:", "prose:", "panics:"} {
		if a, b := reportLine(before, line), reportLine(after, line); a != b {
			t.Errorf("a hidden copy changed the report:\n  before: %s\n  after:  %s", a, b)
		}
	}
}

// reportLine returns the line of a budget report that starts with prefix.
func reportLine(report, prefix string) string {
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}
