package saga

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCurrentDesignContentDigestsTrackAuthoredContentOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "digest.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"digest","title":"Digest","source":{"repository":"https://example.test/app.git","base":"main","head":"HEAD"}}`)
	writeTestFile(t, filepath.Join(root, "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, "overview.fragment", "content.md"), "Narrative.\n")
	writeTestFile(t, filepath.Join(root, "___design", "architecture.chapter", "chapter.json"), `{"version":2,"id":"architecture","title":"Architecture"}`)
	fragmentDir := filepath.Join(root, "___design", "architecture.chapter", "flow.fragment")
	writeTestFile(t, filepath.Join(fragmentDir, "fragment.json"), `{"version":2,"id":"flow","title":"Flow","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(fragmentDir, "content.md"), "# Flow {#flow}\n\nOriginal.\n")
	writeTestFile(t, filepath.Join(fragmentDir, "___landmarks", "flow.landmark", "landmark.json"), `{"version":2,"id":"flow","label":"Flow","selector":{"type":"heading","heading_id":"flow"}}`)

	document, validation, err := Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	before, err := CurrentDesignContentDigests(document)
	if err != nil {
		t.Fatal(err)
	}
	chapter := ChapterTarget("digest", "architecture")
	fragment := FragmentTarget("digest", "flow")
	landmark := LandmarkTarget("digest", "flow", "flow")
	for _, target := range []string{chapter, fragment, landmark} {
		if !strings.HasPrefix(before[target], "sha256:") || len(before[target]) != 71 {
			t.Fatalf("digest[%s] = %q", target, before[target])
		}
	}
	if _, exists := before[FragmentTarget("digest", "overview")]; exists {
		t.Fatal("root narrative was indexed as technical design")
	}

	writeTestFile(t, filepath.Join(fragmentDir, "___diffs", "evidence.json"), `{"version":2,"diffs":[]}`)
	document, _, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	withEvidence, err := CurrentDesignContentDigests(document)
	if err != nil {
		t.Fatal(err)
	}
	if withEvidence[chapter] != before[chapter] || withEvidence[fragment] != before[fragment] || withEvidence[landmark] != before[landmark] {
		t.Fatal("review evidence changed authored design digests")
	}

	if err := os.WriteFile(filepath.Join(fragmentDir, "content.md"), []byte("# Flow {#flow}\n\nRevised.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	document, _, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	after, err := CurrentDesignContentDigests(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{chapter, fragment, landmark} {
		if after[target] == before[target] {
			t.Fatalf("authored content change did not invalidate %s", target)
		}
	}

	got, ok, err := CurrentDesignContentDigest(document, fragment)
	if err != nil || !ok || got != after[fragment] {
		t.Fatalf("lookup = %q, %v, %v", got, ok, err)
	}
}

func TestCurrentDesignContentDigestsCanonicalEmbeddedVisualTargets(t *testing.T) {
	document, fixture := loadVisualDigestFixture(t)

	got, err := CurrentDesignContentDigests(document)
	if err != nil {
		t.Fatal(err)
	}
	// These vectors freeze the canonical byte contract. Each part is prefixed
	// by its uint64 big-endian byte length after the versioned domain:
	//   Item  = canonical Item manifest JSON, exact slide asset bytes
	//   Slide = canonical Slide manifest JSON, exact entrypoint bytes,
	//           JSON array of rank/path-ordered Item-manifest digests
	//   Deck  = canonical Deck manifest JSON, JSON array of ordered Slide digests
	want := map[string]string{
		fixture.deckTarget:   "sha256:d2379dda409e9c94ebbadcb92a08c3febc4f4fd3d45182d40097ee6fa442a5af",
		fixture.slideTarget:  "sha256:23a0a2df8805f654a9db113ea3179e9cca906dbdb6a10deb875e20d991a9aa01",
		fixture.firstTarget:  "sha256:49337c1dcfea5d14d24b5aaa33aac84a9851f73781e1f948f9291349c72e0430",
		fixture.secondTarget: "sha256:03c85e9f512d026f7a2ab2e549fc832d64b5e4471b28f22b6e350a58a6bbbc7c",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("embedded visual digest vectors changed:\n got: %#v\nwant: %#v", got, want)
	}

	// Callers cannot change the canonical order by rearranging model slices;
	// rank and compact manifest path are the authored deck order.
	items := document.Decks[0].Slides[0].Items
	items[0], items[1] = items[1], items[0]
	reordered, err := CurrentDesignContentDigests(document)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reordered, got) {
		t.Fatalf("model slice order changed canonical visual digests:\nbefore: %#v\n after: %#v", got, reordered)
	}

	// JSON layout and object-key order are not content. Reloading equivalent
	// Item bytes must reproduce the typed canonical manifest digest.
	writeTestFile(t, fixture.firstManifest, `{
  "selector": {"element_id": "first", "type": "element"},
  "description": "The first visual item.",
  "label": "First",
  "kind": "node",
  "rank": 10,
  "slide": "flow",
  "id": "first",
  "version": 4
}`)
	reloaded, validation, err := Load(document.Root)
	if err != nil || !validation.Valid {
		t.Fatalf("reload: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	after, err := CurrentDesignContentDigests(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, got) {
		t.Fatalf("non-canonical manifest formatting changed digests:\nbefore: %#v\n after: %#v", got, after)
	}
}

func TestEmbeddedVisualDigestsPropagateOnlyAuthoredInputs(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Saga, visualDigestFixture)
		changed func(visualDigestFixture) map[string]bool
	}{
		{
			name: "item manifest",
			mutate: func(document *Saga, _ visualDigestFixture) {
				document.Decks[0].Slides[0].Items[0].Description = "A revised first visual item."
			},
			changed: func(f visualDigestFixture) map[string]bool {
				return map[string]bool{f.firstTarget: true, f.slideTarget: true, f.deckTarget: true}
			},
		},
		{
			name: "slide asset",
			mutate: func(_ *Saga, f visualDigestFixture) {
				writeTestFile(t, f.assetPath, `<svg xmlns="http://www.w3.org/2000/svg"><g id="first"/><g id="second"/><text>revised</text></svg>`)
			},
			changed: func(f visualDigestFixture) map[string]bool {
				return map[string]bool{f.firstTarget: true, f.secondTarget: true, f.slideTarget: true, f.deckTarget: true}
			},
		},
		{
			name: "slide manifest",
			mutate: func(document *Saga, _ visualDigestFixture) {
				document.Decks[0].Slides[0].Takeaway = "A revised visual takeaway."
			},
			changed: func(f visualDigestFixture) map[string]bool {
				return map[string]bool{f.slideTarget: true, f.deckTarget: true}
			},
		},
		{
			name: "deck manifest",
			mutate: func(document *Saga, _ visualDigestFixture) {
				document.Decks[0].Objective = "A revised deck objective."
			},
			changed: func(f visualDigestFixture) map[string]bool {
				return map[string]bool{f.deckTarget: true}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, fixture := loadVisualDigestFixture(t)
			before, err := CurrentDesignContentDigests(document)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(document, fixture)
			after, err := CurrentDesignContentDigests(document)
			if err != nil {
				t.Fatal(err)
			}
			changed := test.changed(fixture)
			for target, beforeDigest := range before {
				if (after[target] != beforeDigest) != changed[target] {
					t.Errorf("digest change for %s = %v, want %v", target, after[target] != beforeDigest, changed[target])
				}
			}
		})
	}
}

func TestEmbeddedVisualDigestsExcludeEvidenceAndReviewOverlays(t *testing.T) {
	document, _ := loadVisualDigestFixture(t)
	before, err := CurrentDesignContentDigests(document)
	if err != nil {
		t.Fatal(err)
	}

	deck := document.Decks[0]
	slide := deck.Slides[0]
	item := slide.Items[0]
	item.Diffs = []DiffFile{{Version: ComponentVersion, Diffs: []DiffReference{{URI: "saga-diff://overlay"}}}}
	item.HasDiffs = true
	item.Reviews = []Review{{ID: "item-comment", Body: "Comment overlay"}}
	slide.Reviews = []Review{{ID: "slide-approval", State: "approved", Body: "Approval overlay"}}
	deck.Reviews = []Review{{ID: "deck-review", Body: "Derived review overlay"}}
	document.Claims = []Claim{{ID: "claim-overlay", Target: item.Target, Statement: "Claim overlay"}}
	document.Verifications = []Verification{{ID: "verification-overlay", Claim: "claim-overlay", Summary: "Verification overlay"}}
	document.Threads = []*Thread{{ID: "thread-overlay", Target: item.Target, Messages: []*Message{{ID: "comment-overlay"}}}}
	document.DiffReviews = []DiffReview{{ID: "diff-review-overlay", State: "reviewed"}}

	after, err := CurrentDesignContentDigests(document)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("diff, claim, verification, comment, approval, or review overlays changed visual digests:\nbefore: %#v\n after: %#v", before, after)
	}
}

type visualDigestFixture struct {
	deckTarget    string
	slideTarget   string
	firstTarget   string
	secondTarget  string
	firstManifest string
	assetPath     string
}

func loadVisualDigestFixture(t *testing.T) (*Saga, visualDigestFixture) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "visual-digest.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"visual-digest","title":"Visual digest","source":{"repository":"https://example.test/app.git","base":"main","head":"HEAD"}}`)

	bundle := filepath.Join(root, EmbeddedSlidesDir, "architecture.deck")
	deckTarget := DeckTarget("visual-digest", "architecture")
	deckName, err := FlatDeckFilename(deckTarget, 20)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(bundle, deckName), `{"version":4,"id":"architecture","title":"Architecture","role":"change","rank":20,"objective":"Explain the visual architecture."}`)

	slideTarget := SlideTarget("visual-digest", "flow")
	slideName, err := FlatSlideFilename(deckTarget, slideTarget, 30)
	if err != nil {
		t.Fatal(err)
	}
	assetName, err := FlatSlideAssetFilename(slideName, ".svg")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(bundle, slideName), fmt.Sprintf(`{"version":4,"id":"flow","deck":"architecture","title":"Visual flow","rank":30,"intent":"explain","layout":"diagram","media_type":"image/svg+xml","entrypoint":%q,"takeaway":"The visual flow is explicit.","reading_order":["first","second"]}`, assetName))
	assetPath := filepath.Join(bundle, assetName)
	writeTestFile(t, assetPath, `<svg xmlns="http://www.w3.org/2000/svg"><g id="first"/><g id="second"/></svg>`)

	firstTarget := ItemTarget("visual-digest", "flow", "first")
	firstName, err := FlatItemFilename(slideTarget, firstTarget, 10)
	if err != nil {
		t.Fatal(err)
	}
	firstManifest := filepath.Join(bundle, firstName)
	writeTestFile(t, firstManifest, `{"version":4,"id":"first","slide":"flow","rank":10,"kind":"node","label":"First","description":"The first visual item.","selector":{"type":"element","element_id":"first"}}`)

	secondTarget := ItemTarget("visual-digest", "flow", "second")
	secondName, err := FlatItemFilename(slideTarget, secondTarget, 20)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(bundle, secondName), `{"version":4,"id":"second","slide":"flow","rank":20,"kind":"edge","label":"Second","description":"The second visual item.","selector":{"type":"element","element_id":"second"}}`)

	document, validation, err := Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load fixture: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	return document, visualDigestFixture{
		deckTarget: deckTarget, slideTarget: slideTarget, firstTarget: firstTarget, secondTarget: secondTarget,
		firstManifest: firstManifest, assetPath: assetPath,
	}
}
