# shanty

shanty is a terminal application for playing music from a Subsonic or Navidrome
server that you run yourself.

It lists the artists on your server, then the albums by an artist you choose,
then the tracks on an album. When you select a track, shanty plays it and
reports the play back to your server, so your listening history stays on the
machine you control.

shanty does not store music on your computer, download files, or keep a local
library. The audio is decoded and played by mpv, a separate media player that
shanty starts and controls.

## Getting started

- **[Installing](Installing)** — how to install shanty and the mpv media player it requires
- **[Your first run](Your-first-run)** — connecting shanty to your server for the first time
- **[Keys](Keys)** — every keyboard shortcut, grouped by what it does
- **[Commands](Commands)** — what each of shanty's six commands does

## Reference

- **[Credentials](Credentials)** — the four ways to authenticate with your server, and which to prefer
- **[The doctor](The-doctor)** — what each check in `shanty doctor` examines
- **[Where it puts things](Where-it-puts-things)** — the four directories shanty writes to
- **[Security](Security)** — how shanty stores your password, and what it connects to

## Problems

- **[Troubleshooting](Troubleshooting)** — the errors you are most likely to see, and what to do about them

---

*These pages are kept in the project repository, in its `wiki/` directory, and
published from there automatically. If you edit a page here it will be replaced
the next time they are published, so please send a pull request instead.*
