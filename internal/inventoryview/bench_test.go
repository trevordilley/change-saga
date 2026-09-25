package inventoryview

import (
	"fmt"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// syntheticInventory builds n Components, n/8 Systems of 8 members each and
// one implementation Item per System and per Component across 10 features.
func syntheticInventory(n int) (*saga.Saga, *requirements.Inventory) {
	inv := &requirements.Inventory{}
	for i := 0; i < n; i++ {
		inv.Records = append(inv.Records, record("component", fmt.Sprint("c", i), []requirements.TechnicalRevision{revision("r1", nil)}, "active", false))
	}
	features := map[string][]*saga.Item{}
	for s := 0; s < n/8; s++ {
		members := []Pin{}
		for m := 0; m < 8; m++ {
			members = append(members, pin("component", fmt.Sprint("c", s*8+m), "r1"))
		}
		inv.Records = append(inv.Records, record("system", fmt.Sprint("s", s), []requirements.TechnicalRevision{revision("r1", nil, members...)}, "active", false))
		sys := pin("system", fmt.Sprint("s", s), "r1")
		f := fmt.Sprint("f", s%10)
		features[f] = append(features[f], item(f, "s", fmt.Sprint("sys", s), &sys))
	}
	for i := 0; i < n; i++ {
		c := pin("component", fmt.Sprint("c", i), "r1")
		f := fmt.Sprint("f", i%10)
		features[f] = append(features[f], item(f, "s", fmt.Sprint("comp", i), &c))
	}
	return document(features, nil), inv
}

func BenchmarkBuildIndex(b *testing.B) {
	for _, n := range []int{100, 1000, 8000} {
		doc, inv := syntheticInventory(n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				Build(doc, inv)
			}
		})
	}
}

func BenchmarkUsesDeep(b *testing.B) {
	for _, n := range []int{100, 1000, 8000} {
		doc, inv := syntheticInventory(n)
		ix := Build(doc, inv)
		target := ns + "component:c0"
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if page := ix.Uses(target, UseOptions{Depth: MaxDepth}); page.Total != 3 {
					b.Fatalf("uses: %d", page.Total)
				}
			}
		})
	}
}

func BenchmarkCountsAllRecords(b *testing.B) {
	for _, n := range []int{100, 1000, 8000} {
		doc, inv := syntheticInventory(n)
		ix := Build(doc, inv)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for _, r := range inv.Records {
					ix.Counts(r.Target)
				}
			}
		})
	}
}

func BenchmarkFeatureScope(b *testing.B) {
	for _, n := range []int{100, 1000, 8000} {
		doc, inv := syntheticInventory(n)
		ix := Build(doc, inv)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ix.FeatureScope(doc.Features[0])
			}
		})
	}
}
