# shanty.
#
# `make check` is the gate: it is what CI runs and what must pass before a
# commit. Everything else here is a component of it or a convenience.

BINARY := shanty
GOFLAGS := -mod=readonly

.PHONY: help lint vet test race budgets standard check build crossbuild clean

help:
	@echo "make check       lint, vet, race tests, budgets, standard - the gate"
	@echo "make test        go test"
	@echo "make race        go test -race"
	@echo "make budgets     the PLAN.md §0 budgets"
	@echo "make standard    conformance against ../agent-context, if present"
	@echo "make build       build $(BINARY) for this machine"
	@echo "make crossbuild  every platform the release ships"

lint:
	gofmt -l . | grep -v '^vendor/' | (! grep .) || { echo "gofmt -w the files above"; exit 1; }

vet:
	@if ls ./*/*.go ./*.go >/dev/null 2>&1 || [ -d cmd ]; then go $(GOFLAGS) vet ./...; else echo "no packages yet"; fi

test:
	@if ls ./*/*.go ./*.go >/dev/null 2>&1 || [ -d cmd ]; then go $(GOFLAGS) test ./...; else echo "no packages yet"; fi

# The race detector is not optional here: a TUI, a persistent child process and
# an HTTP client is three sources of concurrency, and a data race in that mix
# shows up as a rendering artifact nobody can reproduce.
race:
	@if ls ./*/*.go ./*.go >/dev/null 2>&1 || [ -d cmd ]; then go $(GOFLAGS) test -race ./...; else echo "no packages yet"; fi

budgets:
	@sh scripts/budgets.sh

# The development standard this project is held to lives outside it, so that
# two projects cannot drift into two standards. Absent, this is skipped rather
# than failed: the checkout is allowed to be standalone.
standard:
	@if [ -f ../agent-context/check.sh ]; then sh ../agent-context/check.sh .; \
	 else echo "../agent-context not present; skipped"; fi

# `standard` is in here rather than beside it: a conformance check nothing
# blocks on is the failure it exists to catch, committed one level up.
check: lint vet race budgets standard

build:
	CGO_ENABLED=0 go $(GOFLAGS) build -trimpath -ldflags '-s -w' -o $(BINARY) ./cmd/$(BINARY)

# Every platform the release ships, compiled on every check, because a build
# that only ever runs on the maintainer's machine is a claim about one machine.
crossbuild:
	@for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
	    echo "  $$t"; \
	    GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 \
	        go $(GOFLAGS) build -trimpath -o /dev/null ./... || exit 1; \
	done

clean:
	rm -f $(BINARY)
	rm -rf dist
