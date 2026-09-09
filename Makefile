# shanty.
#
# `make check` is the gate: it is what CI runs and what must pass before a
# commit. Everything else here is a component of it or a convenience.

BINARY  := shanty
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)

# Exported rather than spliced into each recipe: go reads GOFLAGS from the
# environment itself, and `go vet` puts the flag before the
# subcommand, where go does not look for it.
export GOFLAGS := -mod=readonly

.PHONY: help lint vet test race budgets standard check build install-binary crossbuild clean \
	vendor srpm release release-tag copr milestone

help:
	@echo "make check       lint, vet, race tests, budgets, standard - the gate"
	@echo "make test        go test"
	@echo "make race        go test -race"
	@echo "make budgets     the PLAN.md §0 budgets"
	@echo "make standard    conformance against ../agent-context, if present"
	@echo
	@echo "make release VERSION=x.y.z   bump the spec and open the pull request"
	@echo "make release-tag             tag the merged bump; everything else follows"
	@echo "make srpm                    build an SRPM here, the way Copr will"
	@echo "make copr                    ask Copr to build the tag, if the webhook missed it"
	@echo "make build       build $(BINARY) for this machine"
	@echo "make install-binary  build it and put it on PATH"
	@echo "make crossbuild  every platform the release ships"

lint:
	gofmt -l . | grep -v '^vendor/' | (! grep .) || { echo "gofmt -w the files above"; exit 1; }

vet:
	go vet ./...
	# The integration test is behind a build tag, so the line above never
	# compiles it. Vetting it needs no server and catches a change to a
	# signature it uses, which is how it has been broken before.
	go vet -tags=integration ./...

test:
	go test ./...

# The race detector is not optional here: a TUI, a persistent child process and
# an HTTP client is three sources of concurrency, and a data race in that mix
# shows up as a rendering artifact nobody can reproduce.
race:
	go test -race ./...

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

# The same flags the release uses, version string included, so what is measured
# here is what ships.
build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/$(BINARY)

# Every platform the release ships, compiled on every check, because a build
# that only ever runs on the maintainer's machine is a claim about one machine.
crossbuild:
	@for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
	    echo "  $$t"; \
	    GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 \
	        go build -trimpath -o /dev/null ./... || exit 1; \
	done

# Builds shanty and copies it to ~/.local/bin.
install-binary: build
	install -Dm755 $(BINARY) $(HOME)/.local/bin/$(BINARY)
	@echo "installed to ~/.local/bin/$(BINARY)"
	@$(HOME)/.local/bin/$(BINARY) completions install || true
	@case ":$$PATH:" in \
	    *":$(HOME)/.local/bin:"*) ;; \
	    *) echo; echo "~/.local/bin is not on your PATH. Add it:"; \
	       echo '       export PATH="$$HOME/.local/bin:$$PATH"' ;; \
	esac
	@command -v mpv >/dev/null 2>&1 || { echo; \
	    echo "mpv is not installed, and shanty plays through it."; \
	    echo "       dnf install mpv · apt install mpv · brew install mpv"; }
	@echo; echo "next: $(BINARY)"

clean:
	rm -f $(BINARY)
	rm -rf dist

# --- packaging ---------------------------------------------------------------

# Copr build roots have no network, so the dependencies are committed.
vendor:
	go mod vendor
	@echo "vendor/ regenerated; commit it with the go.mod change that needed it"

# An SRPM from the working tree, laid out the way GitHub's tag tarball is, so
# what is tested here is what Copr will build.
srpm:
	@command -v rpmbuild >/dev/null || { echo "rpm-build is not installed"; exit 1; }
	@v=$$(sed -n 's/^Version:[[:space:]]*//p' packaging/$(BINARY).spec); \
	tmp=$$(mktemp -d); \
	mkdir -p "$$tmp/$(BINARY)-$$v" "$$tmp/SOURCES" dist; \
	git ls-files | tar -cf - -T - | tar -xf - -C "$$tmp/$(BINARY)-$$v"; \
	tar -czf "$$tmp/SOURCES/$(BINARY)-$$v.tar.gz" -C "$$tmp" "$(BINARY)-$$v"; \
	rpmbuild -bs packaging/$(BINARY).spec \
	    --define "_sourcedir $$tmp/SOURCES" \
	    --define "_srcrpmdir $$PWD/dist"; \
	rc=$$?; rm -rf "$$tmp"; exit $$rc

# Step one of a release: bump the spec and open the pull request.
#
# Two steps because main requires one. That is not only a rule to satisfy: Copr
# reads Version: from the spec at the tagged commit, so tagging a commit that
# still carries the old number would publish an rpm under it.
release:
	@echo "$(VERSION)" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$$' || { \
	    echo "make release VERSION=x.y.z"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "the tree is not clean"; exit 1; }
	@test "$$(git branch --show-current)" = main || { echo "not on main"; exit 1; }
	@git rev-parse -q --verify "refs/tags/v$(VERSION)" >/dev/null && { \
	    echo "v$(VERSION) is already tagged"; exit 1; } || true
	@test -f docs/review/v$(VERSION).md || { \
	    echo "no review packet at docs/review/v$(VERSION).md; the release will refuse to build"; exit 1; }
	@v=$$(sed -n 's/^Version:[[:space:]]*//p' packaging/$(BINARY).spec); \
	test "$$v" != "$(VERSION)" || { \
	    echo "the spec already says $(VERSION); bumping again would add a second changelog entry for it."; \
	    echo "if the bump is already on main, this release only needs: make release-tag"; exit 1; }
	@command -v gh >/dev/null || { echo "gh is not installed"; exit 1; }
	$(MAKE) check
	git switch -c release/$(VERSION)
	scripts/bump-spec.sh $(VERSION)
	git commit -am "build: $(VERSION)"
	git push -u origin release/$(VERSION)
	gh pr create --fill --title "build: $(VERSION)"

# Step two: tag the merged bump. Everything downstream keys off the tag.
release-tag:
	@test "$$(git branch --show-current)" = main || { echo "not on main"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "the tree is not clean"; exit 1; }
	@git fetch -q origin main
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { \
	    echo "main is not level with origin"; exit 1; }
	@test -f .copr/Makefile || { echo "no .copr/Makefile; Copr would have nothing to run"; exit 1; }
	@v=$$(sed -n 's/^Version:[[:space:]]*//p' packaging/$(BINARY).spec); \
	git rev-parse -q --verify "refs/tags/v$$v" >/dev/null && { \
	    echo "v$$v is already tagged; did the bump pull request get merged?"; exit 1; } || true; \
	test -f "docs/review/v$$v.md" || { echo "no review packet for v$$v"; exit 1; }; \
	git tag "v$$v" && git push origin "v$$v" && \
	echo && echo "  https://github.com/bspeelm/$(BINARY)/actions" && \
	echo "  https://copr.fedorainfracloud.org/coprs/bspeelman/$(BINARY)/builds/"
	@$(MAKE) --no-print-directory milestone || true

# Ask Copr to build the tag. Only needed if the webhook missed one.
copr:
	@v=$$(sed -n 's/^Version:[[:space:]]*//p' packaging/$(BINARY).spec); \
	git rev-parse -q --verify "refs/tags/v$$v" >/dev/null || { \
	    echo "v$$v is not tagged"; exit 1; }; \
	git cat-file -e "v$$v:.copr/Makefile" 2>/dev/null || { \
	    echo "v$$v has no .copr/Makefile"; exit 1; }; \
	copr-cli buildscm $(BINARY) \
	    --clone-url https://github.com/bspeelm/$(BINARY) \
	    --commit "v$$v" \
	    --spec packaging/$(BINARY).spec \
	    --type git \
	    --method make_srpm \
	    --nowait

# Reports what is still open on the milestone and prints the command to close
# it. Never fails: a release that has happened cannot be un-happened by a
# bookkeeping complaint.
milestone:
	@command -v gh >/dev/null || exit 0; \
	v=$$(sed -n 's/^Version:[[:space:]]*//p' packaging/$(BINARY).spec); \
	gh issue list --milestone "v$${v%.*} — a daily driver" --state open 2>/dev/null | head -5 || true

