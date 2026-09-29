package diagram

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func codeSample() Document {
	d := New()
	d.Elements = []Element{{ID: "invoke", Kind: "code", Label: "Create an order", Language: "TypeScript", Code: "const order = await client.orders.create({\n\tcustomer: \"cus_123\",\n\titems: [{ sku: \"book\", quantity: 1 }],\n});\n\nconsole.log(order.id);", X: 80, Y: 100, Width: 1080, Height: 420, Style: "code", LineNumbers: true, HighlightLines: []int{2, 3}}}
	return d
}

func TestCodeExampleRenderPreservesSourceAndIsSafe(t *testing.T) {
	d := codeSample()
	d.Elements[0].Code += "\n// <script>alert(1)</script> & exact source"
	first, err := Render(d, Options{Title: "API usage"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(d, Options{Title: "API usage"})
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("nondeterministic render: %v", err)
	}
	for _, want := range []string{`data-diagram-kind="code"`, `data-language="TypeScript"`, MonoFontPath, `class="diagram-code"`, `xml:space="preserve"`, `>    customer: "cus_123",</tspan>`, `data-code-line="5"`, `data-code-highlight="2"`, `&lt;script&gt;alert(1)&lt;/script&gt; &amp; exact source`} {
		if !bytes.Contains(first, []byte(want)) {
			t.Errorf("missing %s", want)
		}
	}
	if bytes.Contains(first, []byte("<script>")) {
		t.Fatal("source became markup")
	}
	if d.Elements[0].Code != codeSample().Elements[0].Code+"\n// <script>alert(1)</script> & exact source" {
		t.Fatal("render changed source")
	}
}

func TestCodeExampleValidationAndOverflow(t *testing.T) {
	cases := map[string]func(*Element){
		"nonblank":               func(e *Element) { e.Code = " \n " },
		"at most 16000":          func(e *Element) { e.Code = strings.Repeat("x", MaxCodeRunes+1) },
		"at most 100 lines":      func(e *Element) { e.Code = strings.Repeat("x\n", MaxCodeLines) },
		"positive width":         func(e *Element) { e.Width = 0 },
		"only valid on code":     func(e *Element) { e.Kind = "text" },
		"unwrapped":              func(e *Element) { e.Wrap = true },
		"single-line":            func(e *Element) { e.Language = "go\nscript" },
		"control characters":     func(e *Element) { e.Code = "call(\x00)" },
		"unique 1-based":         func(e *Element) { e.HighlightLines = []int{2, 2} },
		"line numbers within":    func(e *Element) { e.HighlightLines = []int{99} },
		"code overflow: line":    func(e *Element) { e.Width = 150 },
		"code overflow: 6 lines": func(e *Element) { e.Height = 30 },
	}
	for want, change := range cases {
		t.Run(want, func(t *testing.T) {
			d := codeSample()
			change(&d.Elements[0])
			_, err := Render(d, Options{})
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want %q; got %v", want, err)
			}
		})
	}
	if got := codeLines("a\tb\r\n\tcall()\n\n"); strings.Join(got, "|") != "a   b|    call()||" {
		t.Fatalf("line normalization: %#v", got)
	}
}

func TestCodeExampleSchemaDescribeAndEdit(t *testing.T) {
	d := codeSample()
	data, err := Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := compileSchema(t).Validate(value); err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	description := Describe(decoded, 0, 100)
	if len(description.Elements) != 1 || description.Elements[0].Code != d.Elements[0].Code || description.Elements[0].Language != "TypeScript" {
		t.Fatalf("lost source: %#v", description)
	}
	var out strings.Builder
	description.WriteText(&out)
	if !strings.Contains(out.String(), "Code examples:") || !strings.Contains(out.String(), "client.orders.create") {
		t.Fatal(out.String())
	}
	set, _ := json.Marshal(map[string]any{"code": "client.close();", "language": "JavaScript", "highlight_lines": nil})
	edited, _, err := Edit(d, []Operation{{Op: "update", ID: "invoke", Set: set}})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Elements[0].Code != "client.close();" || len(edited.Elements[0].HighlightLines) != 0 || d.Elements[0].Code == edited.Elements[0].Code {
		t.Fatal("edit did not preserve independent source")
	}
	if _, err := Render(edited, Options{}); err != nil {
		t.Fatal(err)
	}
	// A new kind is rejected by old readers, but unrelated diagrams retain their SVG bytes.
	if bytes.Contains(mustRender(t, sample()), []byte(MonoFontPath)) {
		t.Fatal("added code font to an unrelated diagram")
	}
}

func mustRender(t *testing.T, d Document) []byte {
	t.Helper()
	svg, err := Render(d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return svg
}
