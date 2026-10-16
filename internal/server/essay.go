package server

import "html/template"

// Abstracts may coexist with other Essays and their own bodies. Keep their
// heading and footnote IDs scoped to both the owner and the presentation slot.
func essayAbstract(source, owner string) template.HTML {
	if source == "" {
		return ""
	}
	return markdownWithAnchors(source, domID(owner)+"--abstract")
}

const essayTemplates = `
{{define "essay-subtext"}}{{if .}}<p class="essay-subtext" data-essay-subtext>{{.}}</p>{{end}}{{end}}
{{define "essay-abstract"}}{{if .}}<section class="essay-abstract" data-essay-abstract aria-label="Abstract"><div class="fragment-markdown">{{.}}</div></section>{{end}}{{end}}
`
