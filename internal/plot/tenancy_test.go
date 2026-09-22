package plot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

// Staff at one builder must not read another builder's plot detail.
//
// The map itself is public, so a plot's number, area and status are not
// secrets. What is behind this handler is: the owner's name, the site office's
// private notes, the owner's unpaid dues, and staff-only documents such as the
// sale deed. Guarding only the mutations left this read open to any staff
// account at any builder that could guess a plot id.
func TestStaffFromAnotherBuilderCannotReadPlotDetail(t *testing.T) {
	f := newFixture(t)

	// Give the plot something worth stealing.
	if _, err := f.db.Exec(f.ctx(), `UPDATE plots SET notes = 'private site note' WHERE id = $1`, f.sold); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(f.ctx(), `
		INSERT INTO maintenance_dues (plot_id, period_label, amount_due, amount_paid)
		VALUES ($1, 'One-time maintenance', 19647.50, 0)`, f.sold); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(f.ctx(), `
		INSERT INTO documents (society_id, plot_id, doc_type, title, url, visibility)
		VALUES ($1, $2, 'sale_deed', 'Sale deed scan', 'https://x/deed.pdf', 'staff')`,
		f.a.SocietyID, f.sold); err != nil {
		t.Fatal(err)
	}

	read := func(id auth.Identity) map[string]any {
		t.Helper()
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil), id)
		r.SetPathValue("plotId", f.sold.String())
		rec := httptest.NewRecorder()
		if err := f.h.get(rec, r); err != nil {
			t.Fatalf("get: %v", err)
		}
		var out map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	t.Run("the owning builder sees everything", func(t *testing.T) {
		body := read(ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin))
		if body["ownerName"] == nil {
			t.Error("the owning builder should see the owner's name")
		}
		if body["dues"] == nil {
			t.Error("the owning builder should see the dues")
		}
	})

	t.Run("another builder's staff see none of it", func(t *testing.T) {
		body := read(ident(f.adminB, &f.b.ID, domain.RoleBuilderAdmin))

		if body["ownerName"] != nil {
			t.Errorf("leaked the owner's name to another builder: %v", body["ownerName"])
		}
		if body["notes"] != nil {
			t.Errorf("leaked private site notes to another builder: %v", body["notes"])
		}
		if body["dues"] != nil {
			t.Errorf("leaked the owner's dues to another builder: %v", body["dues"])
		}
		if docs, ok := body["documents"].([]any); ok && len(docs) > 0 {
			t.Errorf("leaked %d documents to another builder, including staff-only ones", len(docs))
		}
		// The plot's public facts stay visible — the map already shows them.
		if body["plotNo"] != "147" {
			t.Errorf("plotNo = %v; the public view should still work", body["plotNo"])
		}
	})

	t.Run("an unrelated owner sees none of it", func(t *testing.T) {
		stranger := testsupport.NewUser(t, f.db, nil, domain.RoleOwner, "stranger2@example.in", "")
		body := read(ident(stranger, nil, domain.RoleOwner))
		if body["ownerName"] != nil || body["dues"] != nil {
			t.Error("leaked owner details to an unrelated owner")
		}
	})
}

func (f fixture) ctx() context.Context { return context.Background() }
