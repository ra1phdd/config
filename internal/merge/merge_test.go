package merge

import "testing"

const (
	mergeService   = "service"
	mergeFieldName = "name"
	mergeNewValue  = "new"
	mergeOverride  = "override"
	mergeBase      = "base"
)

func TestMergeMapsTracksOriginsAndReplacesSubtrees(t *testing.T) {
	t.Parallel()
	dst := map[string]any{mergeService: map[string]any{mergeFieldName: "old", "port": 1}}
	src := map[string]any{mergeService: map[string]any{mergeFieldName: mergeNewValue}}
	origins := map[string]string{}
	Merge(dst, src, "", mergeOverride, origins, map[string]string{}, "decoder")
	service, ok := dst[mergeService].(map[string]any)
	if !ok {
		t.Fatalf("service = %#v", dst["service"])
	}
	if service[mergeFieldName] != mergeNewValue || service["port"] != 1 || origins[mergeService+"."+mergeFieldName] != mergeOverride {
		t.Fatalf("merged = %#v, origins = %#v", dst, origins)
	}
}

func TestMergeReplacementClearsNestedMetadata(t *testing.T) {
	t.Parallel()
	dst := map[string]any{mergeService: map[string]any{mergeFieldName: "old"}}
	origins := map[string]string{mergeService: mergeBase, mergeService + "." + mergeFieldName: mergeBase}
	decoders := map[string]string{mergeService: mergeBase, mergeService + "." + mergeFieldName: mergeBase}

	Merge(dst, map[string]any{mergeService: mergeNewValue}, "", mergeOverride, origins, decoders, mergeOverride)

	if dst[mergeService] != mergeNewValue {
		t.Fatalf("service = %#v, want new", dst[mergeService])
	}
	if len(origins) != 1 || origins[mergeService] != mergeOverride {
		t.Fatalf("origins = %#v, want only the replacement origin", origins)
	}
	if len(decoders) != 1 || decoders[mergeService] != mergeOverride {
		t.Fatalf("decoders = %#v, want only the replacement decoder", decoders)
	}
}
