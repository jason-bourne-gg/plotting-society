package plot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
)

// Every jsonb column must reach the client as JSON, not as a base64 string.
//
// pgx scans jsonb into a byte slice, and encoding/json marshals a plain []byte
// to base64 — so a field typed []byte silently ships "W3sibGFiZWwi..." where
// the client expects an array or object, and the page dies on .map or .points.
// The types are json.RawMessage to prevent that; this asserts the wire format
// rather than the Go type, so the guarantee survives a refactor.
func TestMapShapeIsJSONNotBase64(t *testing.T) {
	f := newFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("societyId", f.a.SocietyID.String())
	rec := httptest.NewRecorder()
	if err := f.h.list(rec, r); err != nil {
		t.Fatal(err)
	}

	var body struct {
		Plots []struct {
			MapShape json.RawMessage `json:"mapShape"`
		} `json:"plots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Plots) == 0 {
		t.Fatal("no plots returned")
	}

	raw := string(body.Plots[0].MapShape)
	if !strings.HasPrefix(raw, "{") {
		t.Fatalf("mapShape is %q — it should be a JSON object. A base64 string here means the field went back to []byte.", raw)
	}

	// And it must actually carry the polygon the map draws.
	var shape struct {
		Points [][2]float64 `json:"points"`
		Sector int          `json:"sector"`
	}
	if err := json.Unmarshal(body.Plots[0].MapShape, &shape); err != nil {
		t.Fatalf("mapShape does not parse as a polygon: %v", err)
	}
	if len(shape.Points) < 3 {
		t.Errorf("polygon has %d points, want at least 3", len(shape.Points))
	}
}

// The plot editor sends mapShape straight back. With []byte, decoding expects
// base64 and rejects the object the client actually sends.
func TestUpsertAcceptsMapShapeAsAnObject(t *testing.T) {
	f := newFixture(t)
	owner := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)

	body := `{"plotNo":"777","status":"available","mapShape":{"points":[[0,0],[10,0],[10,10],[0,10]],"sector":1}}`
	r := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)), owner)
	r.SetPathValue("societyId", f.a.SocietyID.String())

	rec := httptest.NewRecorder()
	if err := f.h.create(rec, r); err != nil {
		t.Fatalf("a mapShape object was rejected: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
}
