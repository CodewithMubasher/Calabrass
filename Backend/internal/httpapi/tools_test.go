package httpapi

import (
	"testing"

	goai "github.com/zendev-sh/goai"
)

// TestMergeToolsKeepsTheBuiltInAndDropsTheOneThatShadowsIt: the built-in keeps
// its name, and the connector that wanted the same name is reported rather than
// quietly listed beside it.
func TestMergeToolsKeepsTheBuiltInAndDropsTheOneThatShadowsIt(t *testing.T) {
	builtin := goai.Tool{Name: "read_file"}
	shadow := goai.Tool{Name: "read_file"}
	own := goai.Tool{Name: "play_music"}

	merged, skipped := mergeTools([]goai.Tool{builtin}, []goai.Tool{shadow, own})

	if len(merged) != 2 {
		t.Fatalf("got %d tools, want the built-in and the one that does not collide", len(merged))
	}
	if merged[0].Name != "read_file" || merged[1].Name != "play_music" {
		t.Errorf("got %s and %s, want read_file and play_music", merged[0].Name, merged[1].Name)
	}
	if len(skipped) != 1 || skipped[0] != "read_file" {
		t.Errorf("skipped = %v, want the shadowing read_file", skipped)
	}
}

func TestMergeToolsWithNothingEitherWay(t *testing.T) {
	merged, skipped := mergeTools(nil, nil)
	if len(merged) != 0 || len(skipped) != 0 {
		t.Errorf("got %d tools and %v skipped, want nothing", len(merged), skipped)
	}
}

func TestMergeToolsAddsEveryToolThatDoesNotCollide(t *testing.T) {
	merged, skipped := mergeTools(nil, []goai.Tool{{Name: "play_music"}, {Name: "search_web"}})
	if len(merged) != 2 {
		t.Fatalf("got %d tools, want both", len(merged))
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none", skipped)
	}
}
