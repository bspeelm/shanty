package tui

import (
	"fmt"
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
func (m Model) chrome() int {
	n := chromeLines
	if m.nowPlaying != "" {
		n++
	}
	if m.mode == modeCommand {
		n += len(m.completions(m.viewWidth()))
	}
	return n
}

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
	b.WriteString(titleStyle.Render(fit("shanty · "+m.heading(), w)))
	b.WriteString("\n" + rule(w) + "\n")
	b.WriteString(m.list(w, visible))
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
	if len(found) == 1 && found[0].argument != "" {
		c := found[0]
		return []string{"  " + c.name + " <" + c.argument + ">   " + c.summary}
	}

	var rows []string
	row := "  "
	for _, c := range found {
		if lipgloss.Width(row)+lipgloss.Width(c.name)+3 > w && row != "  " {
			rows = append(rows, strings.TrimRight(row, " "))
			row = "  "
		}
		row += c.name + "   "
	}
	return append(rows, strings.TrimRight(row, " "))
}

func (m Model) footer(w int) string {
	if m.mode == modeCommand {
		return statusStyle.Render(fit(":"+m.line+"\u2588", w))
	}
	if m.mode == modeFilter {
		return statusStyle.Render(fit("/"+m.filter+"\u2588", w))
	}
	if m.status == "" {
		return faintStyle.Render(fit(help, w))
	}
	oneLine := strings.ReplaceAll(m.status, "\n", " · ")
	return statusStyle.Render(fit(Sanitise(oneLine), w))
}

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
		return fmt.Sprintf("%s%2d. %s", m.star(s.ID), s.Track, Sanitise(s.Title)), clock(time.Duration(s.Duration) * time.Second)
	case ScreenSearch, ScreenStarred:
		r := m.grouped()[i]
		if r.kind == kindHeading {
			return faintStyle.Render(r.name), ""
		}
		return m.star(r.id) + Sanitise(r.name), Sanitise(r.detail)
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

// star is the mark shown before something the server has starred. Everything
// else gets a space, so the names in a list stay in one column whether
// anything is starred or not.
func (m Model) star(id string) string {
	if m.starred[id] {
		return "★"
	}
	return " "
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
	if room := w - lipgloss.Width(right) - 1; room > 0 && lipgloss.Width(left) > room {
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

func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}
