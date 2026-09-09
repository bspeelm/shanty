# shanty — RPM packaging for Copr.
#
# Built from the GitHub source tag rather than repackaging the release binary:
# a distribution package shipping someone else's prebuilt binary is one in name
# only. Copr build roots have no network, so the dependencies are vendored and
# the build runs with -mod=vendor and GOPROXY=off. If either were wrong the
# build would fail here rather than quietly reaching out.

%global goipath github.com/bspeelm/shanty
%global debug_package %{nil}

Name:           shanty
Version:        0.2.1
Release:        1%{?dist}
Summary:        A terminal client for Navidrome and other Subsonic servers

License:        MIT
URL:            https://github.com/bspeelm/shanty
Source0:        %{url}/archive/refs/tags/v%{version}.tar.gz#/%{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.24
BuildRequires:  git-core

# mpv is required, not suggested. shanty decodes no audio itself and hands
# every track to mpv over a socket (ADR-011), so a shanty without one is a
# music player that cannot play music. This is the one thing a package can do
# that `shanty doctor` could only ever report after the fact.
Requires:       mpv >= 0.24.0

%description
shanty lists the artists on your Subsonic or Navidrome server, then the albums
by one, then the tracks on one, and plays what you choose. It reports the play
back to your server so your listening history stays on the machine you control.

It stores no music, downloads nothing and keeps no local library. Audio is
decoded and played by mpv, a separate player shanty starts and controls over a
socket in a directory only you can open, so the credential in a stream URL
never appears in a command line.

%prep
%autosetup -n %{name}-%{version}

%build
export CGO_ENABLED=0
export GOFLAGS="-mod=vendor"
export GOPROXY=off
go build -trimpath -ldflags "-s -w -X main.Version=%{version}" -o %{name} ./cmd/%{name}

# Written by the binary that was just built, so what the package installs is
# what this version completes. A checked-in copy would be a second answer to
# what the commands are.
mkdir -p completions
./%{name} completions bash > completions/%{name}.bash
./%{name} completions zsh  > completions/_%{name}
./%{name} completions fish > completions/%{name}.fish

%install
install -Dpm 0755 %{name} %{buildroot}%{_bindir}/%{name}
# dnf writes these, not shanty. §8 forbids shanty touching a root path and says
# nothing about a package manager doing its own job.
install -Dpm 0644 completions/%{name}.bash %{buildroot}%{_datadir}/bash-completion/completions/%{name}
install -Dpm 0644 completions/_%{name}     %{buildroot}%{_datadir}/zsh/site-functions/_%{name}
install -Dpm 0644 completions/%{name}.fish %{buildroot}%{_datadir}/fish/vendor_completions.d/%{name}.fish

%check
export GOFLAGS="-mod=vendor"
export GOPROXY=off
# The suite starts an in-process HTTP server, which the build root allows.
# Nothing here reaches the network, and nothing here needs mpv: the tests drive
# a stub that is this test binary re-executed.
go test ./...

%files
%license LICENSE
%doc README.md
%{_bindir}/%{name}
# Owned here as well as by bash-completion, zsh and fish. Owning a directory
# jointly is normal; the alternative is requiring three packages for the sake
# of a directory.
%dir %{_datadir}/bash-completion/completions
%dir %{_datadir}/zsh/site-functions
%dir %{_datadir}/fish/vendor_completions.d
%{_datadir}/bash-completion/completions/%{name}
%{_datadir}/zsh/site-functions/_%{name}
%{_datadir}/fish/vendor_completions.d/%{name}.fish

%changelog
* Tue Sep 08 2026 Bryan Speelman <bryspeelm@gmail.com> - 0.2.1-1
- See https://github.com/bspeelm/shanty/releases/tag/v0.2.1

* Tue Sep 08 2026 Bryan Speelman <bryspeelm@gmail.com> - 0.2.0-1
- Initial Copr package.
