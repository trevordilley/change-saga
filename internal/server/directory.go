package server

import (
	"net/http"
	"strconv"
	"strings"
)

// Every section of the reviewer is a place a reader can open, and most of
// those places are a directory: the table of what the section holds. Terms,
// personas, feature flags, epics, and reviews are all read the same way, so
// they are all built the same way here.
//
// A directory is a real table with real headers, rendered by the server. A
// reader with no JavaScript gets that table and a filter that submits ?q= and
// comes back filtered; a reader with JavaScript gets the same table and types
// into the same field, which hides the rows that do not match without a round
// trip. Both filters read one thing — the text the row already shows — so the
// two paths can never disagree about what matches.
//
// A directory counts and never scores. A cell says how many stories an epic
// holds, never whether that is enough; a place nothing has been authored into
// yet states the growth and names the command that acts on it, the way the
// sidebar's gap rows do.

// directoryView is one such table.
type directoryView struct {
	// ID names the table's DOM node, so the filter field can control it.
	ID    string
	Title string
	Lede  string
	// Action is where the plain filter submits: the directory's own path.
	Action string
	// Query is the filter already applied, echoed back into the field.
	Query string
	// Label names the filter field for a reader who cannot see the table.
	Label string
	// Noun and Nouns are what the rows are, for the caption's count.
	Noun    string
	Nouns   string
	Columns []directoryColumn
	Rows    []directoryRow
	// Total is how many rows there are before the filter.
	Total int
	// Empty states the growth when nothing has been authored here yet, and
	// Command is what an author runs to make the first one.
	Empty   string
	Command string
}

// directoryColumn is one heading. Numeric marks a column of counts, which are
// set flush right and never carry a verdict.
type directoryColumn struct {
	Title   string
	Numeric bool
}

// directoryRow is one record. Text is everything the row shows, which is what
// both filters read. A row the filter rules out is hidden rather than dropped,
// so the reader who submitted that filter can still widen it: the hidden
// attribute already does the hiding for a browser running no JavaScript, and
// leaves app.js the whole table to filter over for one that does.
type directoryRow struct {
	Key     string
	Current bool
	Hidden  bool
	Cells   []directoryCell
	Text    string
}

// directoryCell is one value: a word, a link, or a stated gap. Note is the
// smaller second line a cell sometimes carries, such as a record's id beneath
// its title.
type directoryCell struct {
	Text    string
	Href    string
	Note    string
	Numeric bool
	// Gap marks a cell that states growth rather than a value, so it reads
	// as an absence and not as a failure.
	Gap bool
	// Target is the record's URN, kept so the reviewer's own links can find
	// a row the way they find any other node.
	Target string
}

func textCell(text string) directoryCell { return directoryCell{Text: text} }

func linkCell(text, href string) directoryCell {
	return directoryCell{Text: text, Href: href}
}

func countCell(count int) directoryCell {
	return directoryCell{Text: strconv.Itoa(count), Numeric: true}
}

// gapCell states what is not there yet. It is a fact about the record, in the
// same voice the sidebar uses for a place nothing fills.
func gapCell(note string) directoryCell { return directoryCell{Text: note, Gap: true} }

// listCell joins several values into one cell, or states the gap when there
// are none.
func listCell(values []string, empty string) directoryCell {
	if len(values) == 0 {
		return gapCell(empty)
	}
	return textCell(strings.Join(values, ", "))
}

// addRow appends one row and records the text both filters match on.
func (view *directoryView) addRow(row directoryRow) {
	var text []string
	for _, cell := range row.Cells {
		if cell.Text != "" {
			text = append(text, cell.Text)
		}
		if cell.Note != "" {
			text = append(text, cell.Note)
		}
	}
	row.Text = strings.Join(text, " ")
	view.Rows = append(view.Rows, row)
	view.Total++
}

// apply runs the server-side filter. It is the same comparison app.js makes:
// a case-insensitive match on the text the row shows.
func (view *directoryView) apply(query string) {
	view.Query = strings.TrimSpace(query)
	if view.Query == "" {
		return
	}
	needle := strings.ToLower(view.Query)
	for index := range view.Rows {
		view.Rows[index].Hidden = !strings.Contains(strings.ToLower(view.Rows[index].Text), needle)
	}
}

// Matches is how many rows are in view under the current filter.
func (view directoryView) Matches() int {
	if view.Query == "" {
		return len(view.Rows)
	}
	matches := 0
	for _, row := range view.Rows {
		if !row.Hidden {
			matches++
		}
	}
	return matches
}

// Caption is the table's own name and its count: how many rows a reader is
// looking at, and of how many. app.js rewrites it as a reader types.
func (view directoryView) Caption() string {
	noun := view.Nouns
	if view.Total == 1 && view.Query == "" {
		noun = view.Noun
	}
	if view.Query == "" {
		return strconv.Itoa(view.Total) + " " + noun
	}
	return strconv.Itoa(view.Matches()) + " of " + strconv.Itoa(view.Total) + " " + noun
}

// Filtered reports whether a filter is hiding anything right now.
func (view directoryView) Filtered() bool { return view.Query != "" }

// directoryQuery reads the filter a plain browser submitted.
func directoryQuery(r *http.Request) string { return r.URL.Query().Get("q") }
