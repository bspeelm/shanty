#!/bin/sh
# The §0 budgets, asserted. Run by `make budgets`, which `make check` runs,
# which gates every merge.
#
# These assert from the first commit, before there is anything to assert
# against, because a budget introduced once a project is over it is not a
# budget -- it is a negotiation with a number already lost. Every count below
# is allowed to be zero and the script passes; what it will not do is stay
# quiet once one is exceeded.
#
# Changing a number here means changing PLAN.md §0 in the same commit, with a
# reason. Numbers may tighten. They may not silently grow.

set -eu
cd "$(dirname "$0")/.."

MAX_DIRECT_DEPS=10
MAX_MODULES=30
MAX_PANICS=0
MAX_HTTP_PACKAGES=2
MAX_BINARY_BYTES=15728640   # 15 MiB, linux_amd64, stripped
MIN_TEST_RATIO=3            # at least one test line per three code lines

fail=0
over() { fail=1; printf '\n  %s\n' "$1"; }

# --- dependencies ------------------------------------------------------------
# The field this project is entering ranges from 19 modules to 599 crates. The
# budget does not say a dependency is bad; it says one more of them passes a
# human first.

direct=$(awk '/^require \(/{r=1;next} /^\)/{r=0} r&&!/\/\/ indirect/&&NF{n++}
              /^require [^(]/&&!/\/\/ indirect/{n++} END{print n+0}' go.mod)
printf 'direct deps:   %s (budget %s)\n' "$direct" "$MAX_DIRECT_DEPS"
[ "$direct" -le "$MAX_DIRECT_DEPS" ] || over "over the direct dependency budget -- argue it in an ADR first."

if command -v go >/dev/null 2>&1; then
	modules=$(go list -mod=readonly -m all 2>/dev/null | grep -v '^github.com/bspeelm/shanty' | wc -l | tr -d ' ')
	printf 'modules:       %s (budget %s)\n' "$modules" "$MAX_MODULES"
	[ "$modules" -le "$MAX_MODULES" ] || over "over the total module budget."
fi

# --- go.sum is the lockfile --------------------------------------------------
# A second pinning mechanism is a second answer to "what version is this",
# and the two drift silently.
for f in vendor/modules.txt Gopkg.lock glide.lock; do
	[ -e "$f" ] && over "$f exists; go.sum is the only lockfile (§0)."
done
printf 'lockfile:      go.sum only\n'

# --- source counts -----------------------------------------------------------
gofiles=$(find . -name '*.go' -not -path './vendor/*' -not -path './dist/*' 2>/dev/null | wc -l)
if [ "$gofiles" -eq 0 ]; then
	printf 'code:          none yet -- the budgets are armed and waiting\n'
	exit "$fail"
fi

testlines=$(find . -name '*_test.go' -not -path './vendor/*' -exec cat {} + 2>/dev/null | wc -l)
codelines=$(find . -name '*.go' -not -name '*_test.go' -not -path './vendor/*' -exec cat {} + 2>/dev/null | wc -l)

printf 'code:          %s lines\n' "$codelines"
printf 'tests:         %s lines (floor: 1 per %s of code)\n' "$testlines" "$MIN_TEST_RATIO"
if [ "$codelines" -gt 0 ] && [ $((testlines * MIN_TEST_RATIO)) -lt "$codelines" ]; then
	over "under the test floor -- this is a floor, not a target."
fi

# --- panic -------------------------------------------------------------------
# A panic in a TUI takes the terminal with it. Errors are values here.
panics=$(grep -rn 'panic(' --include='*.go' . 2>/dev/null \
	| grep -v '_test.go' | grep -v '^\./vendor/' | wc -l)
printf 'panics:        %s (budget %s)\n' "$panics" "$MAX_PANICS"
[ "$panics" -le "$MAX_PANICS" ] || { over "panic( in non-test code:"; \
	grep -rn 'panic(' --include='*.go' . 2>/dev/null | grep -v '_test.go' | grep -v '^\./vendor/' | sed 's/^/    /'; }

# --- who talks to the network ------------------------------------------------
# "What does this program say on the network" must have a one-package answer.
if command -v go >/dev/null 2>&1; then
	httpkgs=$(go list -mod=readonly -f '{{.ImportPath}} {{join .Imports " "}} {{join .TestImports " "}}' ./... 2>/dev/null \
		| grep -c ' net/http' || true)
	printf 'net/http:      %s packages (budget %s)\n' "$httpkgs" "$MAX_HTTP_PACKAGES"
	if [ "$httpkgs" -gt "$MAX_HTTP_PACKAGES" ]; then
		over "more packages import net/http than the architecture allows (§4):"
		go list -mod=readonly -f '{{.ImportPath}} {{join .Imports " "}}' ./... 2>/dev/null \
			| grep ' net/http' | awk '{print "    " $1}'
	fi
fi

# --- verification has no off switch ------------------------------------------
# §2: a knob that exists gets turned, and gets pasted into setup guides.
tls=$(grep -rln 'InsecureSkipVerify' --include='*.go' . 2>/dev/null | grep -v '^\./vendor/' || true)
if [ -n "$tls" ]; then
	over "TLS verification bypass present -- there is no off switch (ADR-004):"
	echo "$tls" | sed 's/^/    /'
else
	printf 'tls bypass:    none\n'
fi

# --- the shape of the TUI ----------------------------------------------------
# internal/tui renders a model and emits intents; it does no I/O. That rule is
# what makes golden-file testing of every screen possible, and it is the one of
# the three architectural rules that had no command disagreeing with it. Types
# may cross this boundary. Syscalls may not.
if command -v go >/dev/null 2>&1 && [ -d internal/tui ]; then
	tuiio=$(go list -mod=readonly -f '{{.ImportPath}} {{join .Imports " "}}' ./internal/tui/... 2>/dev/null \
		| grep -E ' (net/http|net|os|os/exec)($| )' || true)
	if [ -n "$tuiio" ]; then
		over "internal/tui does I/O, which is architectural rule 1 (§4):"
		echo "$tuiio" | awk '{print "    " $1}' | sort -u
	else
		printf 'tui i/o:       none\n'
	fi
fi

# --- the artifact ------------------------------------------------------------
# Built here rather than measured wherever one happens to be lying around: the
# budget names linux_amd64 stripped, so this builds exactly that and throws it
# away. A number nothing computes is not a budget, and the release is too late
# to find out.
if command -v go >/dev/null 2>&1 && [ -d cmd/shanty ]; then
	tmpbin=$(mktemp) || exit 1
	trap 'rm -f "$tmpbin"' EXIT INT TERM
	if GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
	   go build -mod=readonly -trimpath -ldflags '-s -w' -o "$tmpbin" ./cmd/shanty 2>/dev/null; then
		size=$(wc -c < "$tmpbin")
		printf 'binary:        %s bytes (budget %s)\n' "$size" "$MAX_BINARY_BYTES"
		[ "$size" -le "$MAX_BINARY_BYTES" ] || over "over the binary budget."
	else
		over "the linux_amd64 build failed, so the binary budget could not be counted."
	fi
fi

if [ "$fail" -ne 0 ]; then
	printf '\nDo not argue with a budget. Change PLAN.md §0 and say why.\n'
	exit 1
fi
