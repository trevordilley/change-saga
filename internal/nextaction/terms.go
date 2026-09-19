package nextaction

import (
	"strings"
	"unicode"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/grammar"
)

// terms keeps the project's vocabulary current. A term whose code reference
// went stale, most often because the code it names was renamed, is revised
// to the code that defines it now. A declaration a comparison added that
// looks like new vocabulary is a growth suggestion. Neither names a gate.
func (b *builder) terms() {
	for _, term := range b.status.Terms {
		if !term.Stale || term.State != "active" || len(term.RevisionHeads) != 1 {
			continue
		}
		values := []grammar.Value{grammar.V("term", term.Term), grammar.V("revision", ""), grammar.V("name", term.Name), grammar.V("definition", term.Definition)}
		values = append(values, parents(term.RevisionHeads)...)
		for _, alias := range term.Aliases {
			values = append(values, grammar.V("alias", alias))
		}
		for _, story := range term.Stories {
			values = append(values, grammar.V("story", story))
		}
		for _, record := range term.Records {
			values = append(values, grammar.V("record", record))
		}
		stale := []string{}
		for _, code := range term.Code {
			if code.State == coderesolve.Current && code.Head != nil {
				values = append(values, grammar.V("ref", code.Head.String()))
			} else {
				stale = append(stale, code.Pinned.Path+lineSuffix(code.Pinned.Start, code.Pinned.End))
			}
		}
		values = append(values, grammar.V("ref", ""))
		b.add(Action{
			ID: "stale:term:" + term.Term, Kind: KindCommand, Category: CategoryStale, Area: AreaHealth, Resource: term.Term,
			Reason: "the code that defines term \"" + term.Name + "\" changed at " + strings.Join(stale, ", ") +
				" (renamed?); revise the term to reference the code that defines it now, or retire it",
			Command: ptr(b.invoke("term revise", values...)),
		})
	}
	for _, suggestion := range b.status.NewTerminology {
		name := suggestion.Name
		if suggestion.Container != "" {
			name = suggestion.Container + "." + suggestion.Name
		}
		where := suggestion.Location.Path + lineSuffix(suggestion.Location.Start, suggestion.Location.End)
		b.add(Action{
			ID: "growth:term:" + suggestion.Location.String(), Kind: KindCommand, Category: CategoryGrowth, Resource: suggestion.Location.String(),
			Practice: practiceTerms,
			Reason:   "this change adds " + name + " (" + where + ") and no term names it: this looks like new terminology; define it while the meaning is fresh?",
			Command: ptr(b.invoke("term add", grammar.V("id", termID(suggestion.Name)), grammar.V("name", suggestion.Name),
				grammar.V("definition", ""), grammar.V("ref", suggestion.Location.String()))),
		})
	}
}

func lineSuffix(start, end int) string {
	switch {
	case start == 0:
		return ""
	case start == end:
		return "#L" + itoa(start)
	}
	return "#L" + itoa(start) + "-L" + itoa(end)
}

// termID suggests a stable term ID for an identifier: its words in lower
// case, joined by hyphens.
func termID(name string) string {
	var builder strings.Builder
	runes := []rune(name)
	for index, r := range runes {
		switch {
		case r == '_' || r == '-':
			builder.WriteByte('-')
		case unicode.IsUpper(r):
			if index > 0 && (unicode.IsLower(runes[index-1]) || index+1 < len(runes) && unicode.IsLower(runes[index+1]) && unicode.IsUpper(runes[index-1])) {
				builder.WriteByte('-')
			}
			builder.WriteRune(unicode.ToLower(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
		}
	}
	return strings.Trim(strings.ReplaceAll(builder.String(), "--", "-"), "-")
}
