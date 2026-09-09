package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// chromeLines is the number of rows that are not list rows: the title, two
// rules, the player line and the last row.
const chromeLines = 5

// chrome is how many rows the frame uses for something other than the list.
// The progress bar and the list of matching commands each take one when they
// are showing.
func (m Model) chrome() int { return m.chromeBase() + len(m.artBlock(m.viewWidth())) }

// chromeBase is every row of the frame that is neither the list nor the cover
// art. It does not count the art, so that the decision to show art can be made
// from it without asking itself.
func (m Model) chromeBase() int {
	n := chromeLines
	if m.nowPlaying != "" {
		n++
	}
	if m.mode == modeCommand {
		n += len(m.completions(m.viewWidth()))
	}
	return n
}

// minTracksBesideArt is how much of a track list has to remain for a cover to
// be worth the rows it takes. An album with fewer tracks than this needs only
// its own: eight rows held for tracks that do not exist is a cover given up
// for nothing.
const minTracksBesideArt = 8

const (
	defaultWidth  = 80
	defaultHeight = 24
)

// Bold, faint and reverse only. Colours arrive with themes.
var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	faintStyle    = lipgloss.NewStyle().Faint(true)
	statusStyle   = lipgloss.NewStyle().Bold(true)
)

// View renders the frame. Every string from the server passes through
// Sanitise on the way.
func (m Model) View() string {
	w, h := m.width, m.height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	visible := max(1, h-m.chrome())

	var b strings.Builder
	// A picture the terminal is holding is taken away before a screen that is
	// not showing one is drawn. Writing text over it does not remove it.
	// The picture the terminal is holding is taken away before anything is
	// drawn, whether or not this frame draws one of its own. Going from a
	// cover filling the screen back to one above a track list draws art in
	// both frames, and without this both pictures end up on screen.
	b.WriteString(m.clearArt)
	b.WriteString(titleStyle.Render(fit("shanty · "+m.heading(), w)))
	b.WriteString("\n" + rule(w) + "\n")

	// A cover filling the screen is all there is to see. The list is not drawn
	// under it, because there is no room for one and nothing to scroll.
	if len(m.bigArt) > 0 {
		for _, line := range m.bigArt {
			b.WriteString(line + "\n")
		}
		for range max(0, visible-len(m.bigArt)) {
			b.WriteString("\n")
		}
	} else {
		for _, line := range m.artBlock(w) {
			b.WriteString(line + "\n")
		}
		b.WriteString(m.list(w, visible))
	}
	b.WriteString(rule(w) + "\n")
	b.WriteString(fit(m.player(), w) + "\n")
	if m.nowPlaying != "" {
		b.WriteString(m.progress(w) + "\n")
	}
	if m.mode == modeCommand {
		for _, row := range m.completions(w) {
			b.WriteString(faintStyle.Render(fit(row, w)) + "\n")
		}
	}
	b.WriteString(m.footer(w))
	return b.String()
}

// footer is the last row of the frame. It carries the current message when
// there is one, and the key help otherwise.
//
// Messages arrive with line breaks in them, because the errors that produce
// them put the cause on one line and what to do about it on the next. The
// footer is one row, so the breaks become separators before Sanitise removes
// them and runs the two halves together.
// completions lists the commands the typed line matches. It is what makes the
// command set learnable without a manual, so it wraps rather than cutting off
// the commands that do not fit on one row.
func (m Model) completions(w int) []string {
	found := matching(m.line)
	if len(found) == 0 {
		return []string{"  no command starts with that"}
	}
	// Narrowed to one, it says what the command does. This used to depend on
	// the command taking an argument, so the ones that take none -- `art`,
	// `wiki`, `resume` -- never explained themselves at any point.
	if len(found) == 1 {
		return []string{"  " + spelled(found[0]) + "   " + found[0].summary}
	}

	var rows []string
	row := "  "
	for _, c := range found {
		name := spelled(c)
		if lipgloss.Width(row)+lipgloss.Width(name)+3 > w && row != "  " {
			rows = append(rows, strings.TrimRight(row, " "))
			row = "  "
		}
		row += name + "   "
	}
	return append(rows, strings.TrimRight(row, " "))
}

// spelled is a command as the completion list writes it, with what follows it
// where there is something. A list of bare words says nothing about which of
// them need typing after.
func spelled(c command) string {
	if c.argument == "" {
		return c.name
	}
	return c.name + " <" + c.argument + ">"
}

func (m Model) footer(w int) string {
	if m.mode == modeCommand {
		return statusStyle.Render(fit(":"+m.line+"\u2588", w))
	}
	if m.mode == modeFilter {
		return statusStyle.Render(fit("/"+m.filter+"\u2588", w))
	}
	if m.mode == modeConfirm {
		return statusStyle.Render(fit(Sanitise(m.asking)+"  [y/N]", w))
	}
	if m.status == "" {
		// Editing is a mode, and a mode that cannot be seen is not allowed to
		// change what a key does (ADR-019). The artist and album lists carry
		// no track marks, so this row is the only sign of it there.
		if m.editing.ID != "" {
			return statusStyle.Render(fit(editingHint(m.editing.Name), w))
		}
		return faintStyle.Render(fit(help, w))
	}
	oneLine := strings.ReplaceAll(m.status, "\n", " · ")
	return statusStyle.Render(fit(Sanitise(oneLine), w))
}

// editingHint is the row shown while a playlist is being edited. It names the
// playlist and the three keys that act on it.
func editingHint(name string) string {
	return "adding to " + Sanitise(name) + " — a adds, r removes, e leaves"
}

// blankLike is an empty block the same shape as a rendered cover. The width is
// taken from a padding row rather than the first, which carries an escape
// sequence in every protocol that draws a picture.
func blankLike(lines []string) []string {
	width := lipgloss.Width(lines[len(lines)-1])
	out := make([]string, len(lines))
	for i := range out {
		out[i] = strings.Repeat(" ", width)
	}
	return out
}

// The cover sits in from the left edge the way a list row does, with a gap
// between it and the album's facts.
const (
	artIndent = "   "
	artGap    = "   "
	// artFraming is the blank row above the cover, the blank row below it and
	// the rule that separates the pair from the track list.
	artFraming = 3
)

// artBlock is the cover, the album's facts beside it, and the rule between the
// two and the track list. It is empty when there is no cover.
//
// The facts are here rather than in the list because the space beside a cover
// is otherwise dead: twelve rows of nothing, in the one place on the screen
// where there is something to say about the record.
func (m Model) artBlock(w int) []string {
	art := m.artLines()
	if len(art) == 0 {
		return nil
	}
	facts := m.albumFacts()
	out := make([]string, 0, len(art)+artFraming)
	out = append(out, "")
	for i, line := range art {
		row := artIndent + line
		if i < len(facts) {
			row += artGap + facts[i]
		}
		out = append(out, strings.TrimRight(row, " "))
	}
	return append(out, "", rule(w))
}

// albumFacts is what is worth saying about a record beside its cover. The
// title bar already carries the artist and the album, so these are the things
// that are nowhere else.
func (m Model) albumFacts() []string {
	a := m.album
	facts := []string{titleStyle.Render(Sanitise(a.Name)), Sanitise(a.Artist)}
	if a.Year > 0 {
		facts = append(facts, faintStyle.Render(strconv.Itoa(a.Year)))
	}
	count := plural(len(a.Songs), "track")
	if a.Duration > 0 {
		count += " · " + long(time.Duration(a.Duration)*time.Second)
	}
	return append(facts, faintStyle.Render(count))
}

// artLines is the cover art rows on the screen showing an album's tracks.
// Every other screen shows none: the art belongs to one album, and a list of
// artists is not about any of them.
func (m Model) artLines() []string {
	if m.screen != ScreenTracks || len(m.art) == 0 {
		return nil
	}
	// A cover is worth having only while the list it sits above still shows
	// the album, or enough of it to be worth reading. What it is measured
	// against is the tracks there are: a record with three of them does not
	// need eight rows kept clear, and giving up the cover to hold rows nothing
	// will fill is the picture lost for nothing.
	if m.viewHeight()-m.chromeBase()-len(m.art)-artFraming < min(m.rows(), minTracksBesideArt) {
		return nil
	}
	return m.art
}

// reading reports whether the screen is a page of prose rather than a list.
func (m Model) reading() bool { return m.screen == ScreenWiki && m.topic != "" }

func (m Model) heading() string {
	switch m.screen {
	case ScreenAlbums:
		return Sanitise(m.artist.Name)
	case ScreenTracks:
		return Sanitise(m.album.Artist) + " · " + Sanitise(m.album.Name)
	case ScreenSearch:
		return searchHeading(m.query, m.found)
	case ScreenStarred:
		return starredHeading(m.starredRows)
	case ScreenPlaylists:
		if len(m.playlists) == 0 {
			return "playlists · none yet"
		}
		return fmt.Sprintf("playlists · %s", plural(len(m.playlists), "playlist"))
	case ScreenPlaylist:
		return Sanitise(m.playlist.Name) + " · " + plural(len(m.playlist.Songs), "track")
	case ScreenWiki:
		if m.topic != "" {
			return "wiki · :" + m.topic
		}
		return fmt.Sprintf("wiki · %s", plural(len(commands), "command"))
	case ScreenMessages:
		if len(m.said) == 0 {
			return "messages · nothing said yet"
		}
		return fmt.Sprintf("messages · %s", plural(len(m.said), "message"))
	case ScreenQueue:
		switch {
		case len(m.queued) == 0:
			return "queue"
		case m.queuedAt >= len(m.queued):
			// Every track has been played. Saying "3 of 3" here would name a
			// track as playing when none is.
			return fmt.Sprintf("queue · finished, %s", plural(len(m.queued), "track"))
		}
		return fmt.Sprintf("queue · %d of %d", m.queuedAt+1, len(m.queued))
	}
	return "artists"
}

func (m Model) list(w, visible int) string {
	rows := m.rows()
	if rows == 0 {
		body := "nothing here"
		switch {
		case m.loading:
			body = "loading…"
		case m.filter != "":
			body = "nothing matches " + Sanitise(m.filter)
		}
		return pad(faintStyle.Render(fit("  "+body, w)), visible)
	}

	start := window(m.cursor[m.screen], rows, visible)
	var b strings.Builder
	for i := start; i < min(rows, start+visible); i++ {
		left, right := m.row(i)
		// A page of prose has no row to select, so it is scrolled without a
		// cursor sitting on a sentence.
		if m.reading() {
			b.WriteString(fit(left, w) + "\n")
			continue
		}
		line := fit(gutter(i == m.cursor[m.screen])+columns(left, right, w-2), w)
		if i == m.cursor[m.screen] {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return pad(strings.TrimRight(b.String(), "\n"), visible)
}

// row returns the two halves of one list line: what it is, and its count or
// duration.
func (m Model) row(i int) (string, string) {
	i = m.matches()[i]
	switch m.screen {
	case ScreenArtists:
		a := m.artists[i]
		return m.star(a.ID) + Sanitise(a.Name), plural(a.AlbumCount, "album")
	case ScreenAlbums:
		a := m.artist.Albums[i]
		return m.star(a.ID) + Sanitise(a.Name), plural(a.SongCount, "track")
	case ScreenTracks:
		s := m.album.Songs[i]
		return fmt.Sprintf("%s%s%2d. %s", m.added(s.ID), m.star(s.ID), s.Track, Sanitise(s.Title)),
			clock(time.Duration(s.Duration) * time.Second)
	case ScreenSearch, ScreenStarred:
		r := m.grouped()[i]
		if r.kind == kindHeading {
			return faintStyle.Render(r.name), ""
		}
		return m.star(r.id) + Sanitise(r.name), Sanitise(r.detail)
	case ScreenPlaylists:
		p := m.playlists[i]
		return Sanitise(p.Name), plural(p.SongCount, "track")
	case ScreenPlaylist:
		s := m.playlist.Songs[i]
		return fmt.Sprintf("%s%2d. %s", m.star(s.ID), i+1, Sanitise(s.Title)),
			Sanitise(s.Artist) + " · " + clock(time.Duration(s.Duration)*time.Second)
	case ScreenWiki:
		if m.topic != "" {
			return helpLines(m.topic, m.viewWidth())[i], ""
		}
		c := commands[i]
		name := ":" + c.name
		if c.argument != "" {
			name += " <" + c.argument + ">"
		}
		return name, c.summary
	case ScreenMessages:
		said := m.said[m.saidAt(i)]
		// The message is one row, and some carry the cause on one line and
		// what to do on the next.
		return Sanitise(strings.ReplaceAll(said.text, "\n", " · ")), said.at.Format("15:04:05")
	case ScreenQueue:
		t := m.queued[i]
		mark := "  "
		if i == m.queuedAt {
			mark = "▶ "
		}
		return mark + m.star(t.ID) + Sanitise(t.Title) + " · " + Sanitise(t.Artist), clock(t.Duration)
	}
	return "", ""
}

func (m Model) player() string {
	if m.nowPlaying == "" {
		return faintStyle.Render("nothing playing")
	}
	mark := "▶"
	if m.paused {
		mark = "❚❚"
	}
	left := fmt.Sprintf("%s %s · %s", mark, Sanitise(m.nowPlaying), Sanitise(m.nowArtist))
	right := fmt.Sprintf("%s   vol %d%%", clock(m.position), m.volume)
	if m.duration > 0 {
		right = fmt.Sprintf("%s / %s   vol %d%%", clock(m.position), clock(m.duration), m.volume)
	}
	return columns(left, right, m.viewWidth())
}

// progress is the bar showing how far through the track playback has reached.
// It fills the given width. A track whose length the server did not report
// draws as empty.
func (m Model) progress(w int) string {
	w = max(1, w)
	var done int
	if m.duration > 0 {
		done = int(int64(w) * int64(m.position) / int64(m.duration))
	}
	done = max(0, min(w, done))
	return statusStyle.Render(strings.Repeat("━", done)) +
		faintStyle.Render(strings.Repeat("─", w-done))
}

// added marks a track already in the playlist being edited. Nothing is marked
// when nothing is being edited, so the column is not there at all.
func (m Model) added(id string) string {
	if m.editing.ID == "" {
		return ""
	}
	if m.inPlaylist[id] {
		return "(A) "
	}
	return "    "
}

// star is the mark shown before something the server has starred. Everything
// else gets a space, so the names in a list stay in one column whether
// anything is starred or not.
func (m Model) star(id string) string {
	if m.starred[id] {
		return "★"
	}
	return " "
}

func (m Model) viewHeight() int {
	if m.height > 0 {
		return m.height
	}
	return defaultHeight
}

func (m Model) viewWidth() int {
	if m.width > 0 {
		return m.width
	}
	return defaultWidth
}

const help = "↑↓ move · gg top · G end · enter open · esc back · / filter · : command · space pause · n/p skip · [ ] seek · +/- volume · :q quit"

// window returns the first row to draw, scrolling only enough to keep the
// cursor on screen.
func window(cursor, rows, visible int) int {
	if rows <= visible || cursor < visible/2 {
		return 0
	}
	start := cursor - visible/2
	return min(start, rows-visible)
}

// columns puts left and right on one line of width w, dropping the gap when
// there is no room for both.
// columns puts left at the start of a row and right at its end. A left too
// long to leave room is cut, so that the right is not the part that goes: on
// the messages screen it is the time, which is what makes a log a log.
func columns(left, right string, w int) string {
	// A row with nothing on the right gives the whole width to the left, and
	// only reserves a gap when there is something to keep clear of.
	room := w
	if right != "" {
		room = w - lipgloss.Width(right) - 1
	}
	if room > 0 && lipgloss.Width(left) > room {
		left = fit(left, room)
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// fit truncates s to a display width of w, measured in terminal cells rather
// than characters.
func fit(s string, w int) string {
	if w <= 0 || lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if lipgloss.Width(b.String()+string(r)) > w-1 {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "…"
}

func gutter(selected bool) string {
	if selected {
		return "> "
	}
	return "  "
}

func rule(w int) string { return faintStyle.Render(strings.Repeat("─", max(1, w))) }

// pad extends body to the given number of lines, so the player line stays in
// the same place.
func pad(body string, lines int) string {
	have := strings.Count(body, "\n") + 1
	return body + strings.Repeat("\n", max(1, lines-have+1))
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// long is a duration that may run past an hour, as a record does. A track
// never does, so clock does not carry the hours and this does.
func long(d time.Duration) string {
	if d < time.Hour {
		return clock(d)
	}
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}
