# Copr's "make_srpm" entry point.
#
# Copr clones this repository at the pushed ref and runs
#
#     make -f .copr/Makefile srpm outdir=<some dir>
#
# so the release path is: push a tag -> webhook -> Copr builds. Nothing local,
# and no SRPM uploaded by hand.
#
# The tarball is built from the checkout rather than downloaded from the tag,
# even though the spec's Source0 names that URL. Building from the checkout is
# what makes this work the moment the tag is pushed, rather than only once
# GitHub has finished generating the archive for it.
#
# vendor/ is committed, so neither this nor the build root needs a network.

srpm:
	@command -v rpmbuild >/dev/null || dnf -y install rpm-build
	@command -v git >/dev/null || dnf -y install git
	@test -n "$(outdir)" || { echo "outdir= is required"; exit 1; }
	@mkdir -p "$(outdir)"
	@v=$$(sed -n 's/^Version:[[:space:]]*//p' packaging/shanty.spec); \
	test -n "$$v" || { echo "no Version: in packaging/shanty.spec"; exit 1; }; \
	tmp=$$(mktemp -d); \
	mkdir -p "$$tmp/shanty-$$v" "$$tmp/SOURCES"; \
	git ls-files | tar -cf - -T - | tar -xf - -C "$$tmp/shanty-$$v"; \
	tar -czf "$$tmp/SOURCES/shanty-$$v.tar.gz" -C "$$tmp" "shanty-$$v"; \
	rpmbuild -bs packaging/shanty.spec \
	    --define "_sourcedir $$tmp/SOURCES" \
	    --define "_srcrpmdir $(outdir)"; \
	rc=$$?; rm -rf "$$tmp"; exit $$rc
