package applayout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestFeatureThemeLoaderAndSchema(t *testing.T) {
	root := t.TempDir()
	if themes, err := FeatureThemes(root, "shop"); err != nil || len(themes) != 0 {
		t.Fatalf("legacy: %v %v", themes, err)
	}
	feature, err := WriteFeature(root, FeatureManifest{ID: "core", Title: "Core"})
	if err != nil {
		t.Fatal(err)
	}
	value := FeatureTheme{Schema: FeatureThemeSchemaURL, Version: 5, ID: "reliable", Title: "Reliable", Overview: "# Why\n\nWork survives interruptions.", Features: []string{FeatureURN("shop", feature.ID)}, CreatedAt: time.Now().UTC()}
	path := filepath.Join(root, FeatureThemeRel(value.ID), "theme.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(path, value, true); err != nil {
		t.Fatal(err)
	}
	if themes, err := FeatureThemes(root, "shop"); err != nil || len(themes) != 1 {
		t.Fatalf("load: %v %v", themes, err)
	}
	data, err := os.ReadFile("../../schema/v5/feature-theme.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	schemaJSON, err := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(FeatureThemeSchemaURL, schemaJSON); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(FeatureThemeSchemaURL)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(value)
	var instance any
	_ = json.Unmarshal(encoded, &instance)
	if err := schema.Validate(instance); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*FeatureTheme)
	}{
		{"cross saga", func(v *FeatureTheme) { v.Features = []string{FeatureURN("other", "core")} }},
		{"missing member", func(v *FeatureTheme) { v.Features = []string{FeatureURN("shop", "missing")} }},
		{"duplicate", func(v *FeatureTheme) { v.Features = append(v.Features, v.Features[0]) }},
		{"bare id", func(v *FeatureTheme) { v.Features = []string{"core"} }},
		{"blank overview", func(v *FeatureTheme) { v.Overview = " \n" }},
		{"null members", func(v *FeatureTheme) { v.Features = nil }},
		{"large overview", func(v *FeatureTheme) { v.Overview = strings.Repeat("x", 256*1024+1) }},
		{"mismatched directory", func(v *FeatureTheme) { v.ID = "other" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := value
			test.mutate(&copy)
			if err := store.WriteJSON(path, copy, false); err != nil {
				t.Fatal(err)
			}
			if _, err := FeatureThemes(root, "shop"); err == nil {
				t.Fatal("accepted invalid theme")
			}
		})
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "theme.json")
	if err := store.WriteJSON(outside, value, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := FeatureThemes(root, "shop"); err == nil {
		t.Fatal("accepted symlink manifest")
	}
}
