package cli

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/twentyideas/changesaga/internal/diagram"
)

type whiteboardWarning struct {
	Element string `json:"element"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Reporting only: notes can contain Markdown links, autolinks, references, HTML,
// or plain URLs. Inspect URLs independently of their Markdown presentation and
// never rewrite notes or infer a code selector from a repository URL.
var whiteboardNoteURLs = regexp.MustCompile("https?://[^\\s<>\\\"`]+")

func whiteboardNoteWarnings(doc diagram.Document, repository string) []whiteboardWarning {
	remote := repository
	if strings.HasPrefix(remote, "git@") {
		remote = "https://" + strings.Replace(strings.TrimPrefix(remote, "git@"), ":", "/", 1)
	}
	source, _ := url.Parse(strings.TrimSuffix(remote, ".git"))
	warnings := []whiteboardWarning{}
	for _, e := range doc.Elements {
		for _, raw := range whiteboardNoteURLs.FindAllString(e.Note, -1) {
			link, err := url.Parse(strings.TrimRight(raw, ").,;]"))
			if err != nil {
				continue
			}
			known := false
			switch strings.ToLower(link.Hostname()) {
			case "github.com", "gitlab.com", "bitbucket.org":
				known = len(strings.Split(strings.Trim(link.Path, "/"), "/")) >= 2
			}
			if source != nil && source.Host != "" && strings.EqualFold(link.Host, source.Host) && (link.Path == source.Path || strings.HasPrefix(link.Path, strings.TrimSuffix(source.Path, "/")+"/")) {
				known = true
			}
			if !known {
				continue
			}
			warnings = append(warnings, whiteboardWarning{Element: e.ID, Code: "repository_link_in_note", Message: "Repository link in note: pin exact code in this element's evidence so it opens in the drawer and becomes stale when code changes. Notes remain valid; no link was rewritten."})
			break
		}
	}
	return warnings
}
