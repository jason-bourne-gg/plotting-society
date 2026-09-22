package lead

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// highlights and amenities are jsonb and drive the guest landing page, which
// calls .map on both. As []byte they would arrive base64-encoded and the page
// would throw "(e.highlights ?? []).map is not a function". See the note on
// PublicSociety.
func TestHighlightsAndAmenitiesAreJSONArrays(t *testing.T) {
	f := newFixture(t)

	if _, err := f.db.Exec(f.dbCtx(), `
		UPDATE societies
		   SET highlights = '[{"label":"823 Plots","detail":"Four sectors"}]'::jsonb,
		       amenities  = '["Club House","Swimming Pool"]'::jsonb
		 WHERE id = $1`, f.a.SocietyID); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("slug", f.a.Slug)
	if err := f.handler.publicSocietyBySlug(rec, r); err != nil {
		t.Fatal(err)
	}

	var body struct {
		Highlights json.RawMessage `json:"highlights"`
		Amenities  json.RawMessage `json:"amenities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for name, raw := range map[string]json.RawMessage{
		"highlights": body.Highlights,
		"amenities":  body.Amenities,
	} {
		if !strings.HasPrefix(string(raw), "[") {
			t.Errorf("%s is %q — it should be a JSON array. A base64 string means the field went back to []byte.",
				name, string(raw))
		}
	}

	var highlights []struct {
		Label  string `json:"label"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body.Highlights, &highlights); err != nil {
		t.Fatalf("highlights does not parse as the array the page maps over: %v", err)
	}
	if len(highlights) == 0 || highlights[0].Label == "" {
		t.Errorf("highlights = %+v", highlights)
	}

	var amenities []string
	if err := json.Unmarshal(body.Amenities, &amenities); err != nil {
		t.Fatalf("amenities does not parse as a string array: %v", err)
	}
	if len(amenities) != 2 {
		t.Errorf("amenities = %v", amenities)
	}
}
