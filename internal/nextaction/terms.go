package nextaction

import (
	"strings"
	"unicode"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
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
	b.newTerminology()
}

// newTerminology groups the declarations a comparison added that look like
// new vocabulary by their declaration block, a type's members in one file,
// and offers each block as one question: which of these are words the team
// uses? Each answer defines one term under the domain word the identifier
// spells, referencing the identifier's code.
func (b *builder) newTerminology() {
	suggested := map[string]int{}
	order := []string{}
	blocks := map[string][]livingapp.TermSuggestion{}
	for _, suggestion := range b.status.NewTerminology {
		suggested[suggestion.Suggested]++
		key := suggestion.Location.Path + "#" + suggestion.Container
		if _, ok := blocks[key]; !ok {
			order = append(order, key)
		}
		blocks[key] = append(blocks[key], suggestion)
	}
	for _, key := range order {
		block := blocks[key]
		first, last := block[0].Location, block[0].Location
		names := []string{}
		options := []Option{}
		for _, suggestion := range block {
			if suggestion.Location.Start < first.Start {
				first = suggestion.Location
			}
			if suggestion.Location.End > last.End {
				last = suggestion.Location
			}
			word := firstNonEmptyString(suggestion.Suggested, suggestion.Name)
			id := termID(strings.ReplaceAll(word, " ", "-"))
			if suggested[suggestion.Suggested] > 1 && suggestion.Container != "" {
				// Two blocks suggest the same word; the type tells them apart.
				id = termID(suggestion.Container) + "-" + id
			}
			names = append(names, "\""+word+"\"")
			options = append(options, option("define \""+word+"\"", "a term named \""+word+"\" whose code reference is "+identifier(suggestion)+" at "+suggestion.Location.String(),
				b.invoke("term add", grammar.V("id", id), grammar.V("name", word), grammar.V("definition", ""), grammar.V("ref", suggestion.Location.String()))))
		}
		options = append(options, option("not now", "nothing is recorded; a later comparison that adds such declarations suggests them again"))
		where := first.Path + lineSuffix(first.Start, last.End)
		reason := "this change adds " + identifier(block[0]) + " (" + where + ") and no term names it; is " + names[0] + " new terminology? define it while the meaning is fresh?"
		if len(block) > 1 {
			members := "declarations"
			if block[0].Container != "" {
				members = "members of " + block[0].Container
			}
			reason = "this change adds " + itoa(len(block)) + " " + members + " (" + where + ") that no term names: " + someOf(names, 5) +
				"; define the ones that are project vocabulary while their meaning is fresh?"
		}
		b.add(Action{
			ID: "growth:term:" + key, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaTerms, Resource: first.Path,
			Practice: practiceTerms, Reason: reason,
			Question: question("Which of "+strings.Join(names, ", ")+" are words the team uses, and what does each mean?", NeedProductJudgment, options...),
		})
	}
}

// identifier names a declaration as the code spells it, with its type.
func identifier(suggestion livingapp.TermSuggestion) string {
	if suggestion.Container != "" {
		return suggestion.Container + "." + suggestion.Name
	}
	return suggestion.Name
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
