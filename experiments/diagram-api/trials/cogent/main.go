// A standalone library evaluation, excluded from both production and prototype builds.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"cogentcore.org/core/math32"
	"cogentcore.org/core/svg"
)

func main() {
	input := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><defs><filter id="shadow"><feGaussianBlur stdDeviation="2"/></filter></defs><g id="api" data-item-id="api"><rect x="100" y="100" width="180" height="80" fill="#ffffff"/><text x="120" y="140">API</text></g><path id="request" data-from="api" data-to="db" d="M280 140L400 140"/><g id="db"><circle cx="420" cy="140" r="20"/></g></svg>`
	drawing := svg.NewSVG(math32.Vec2(1280, 720))
	err := drawing.ReadXML(strings.NewReader(input))
	var out bytes.Buffer
	writeErr := drawing.WriteXML(&out, false)
	report := map[string]any{"version": "v0.3.42", "input_bytes": len(input), "output_bytes": out.Len(), "item_id_preserved": strings.Contains(out.String(), `id="api"`), "item_attribute_preserved": strings.Contains(out.String(), `data-item-id="api"`), "connection_preserved": strings.Contains(out.String(), `data-from="api"`), "filter_preserved": strings.Contains(out.String(), "feGaussianBlur")}
	if err != nil {
		report["read_error"] = err.Error()
	}
	if writeErr != nil {
		report["write_error"] = writeErr.Error()
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(b))
	fmt.Println(out.String())
}
