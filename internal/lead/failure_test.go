package lead

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

func TestBehaviourWhenTheDatabaseIsDown(t *testing.T) {
	f := newFixture(t)
	enquiryID := testsupport.NewEnquiry(t, f.db, f.a.SocietyID, &f.plotA, "Guest")
	societyID, slug := f.a.SocietyID, f.a.Slug
	f.db.Close()

	ctx := context.Background()

	if _, err := f.store.PublicBySlug(ctx, slug); err == nil {
		t.Error("PublicBySlug should fail")
	}
	if _, err := f.store.FirstPublic(ctx); err == nil {
		t.Error("FirstPublic should fail")
	}
	if _, err := f.store.Create(ctx, societyID, NewEnquiry{Name: "x", Phone: "9822041190"}); err == nil {
		t.Error("Create should fail")
	}
	if _, err := f.store.List(ctx, societyID, "", 10); err == nil {
		t.Error("List should fail")
	}
	if _, err := f.store.Counts(ctx, societyID); err == nil {
		t.Error("Counts should fail")
	}
	if err := f.store.SetStatus(ctx, enquiryID, f.adminA, "lost", ""); err == nil {
		t.Error("SetStatus should fail")
	}

	if got := status(t, f.handler.publicSociety(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/x", nil))); got != http.StatusInternalServerError {
		t.Errorf("publicSociety status = %d", got)
	}

	bySlug := httptest.NewRequest(http.MethodGet, "/x", nil)
	bySlug.SetPathValue("slug", slug)
	if got := status(t, f.handler.publicSocietyBySlug(httptest.NewRecorder(), bySlug)); got != http.StatusInternalServerError {
		t.Errorf("publicSocietyBySlug status = %d", got)
	}

	enquiry := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(
		`{"societySlug":"`+slug+`","name":"Sagar","phone":"9822041190"}`))
	if got := status(t, f.handler.createEnquiry(httptest.NewRecorder(), enquiry)); got != http.StatusInternalServerError {
		t.Errorf("createEnquiry status = %d", got)
	}

	list := httptest.NewRequest(http.MethodGet, "/x", nil)
	list.SetPathValue("societyId", societyID.String())
	if got := status(t, f.handler.listEnquiries(httptest.NewRecorder(), asStaff(list, f.a.ID, f.adminA))); got != http.StatusInternalServerError {
		t.Errorf("listEnquiries status = %d", got)
	}

	patch := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"status":"lost"}`))
	patch.SetPathValue("enquiryId", enquiryID.String())
	if got := status(t, f.handler.updateEnquiry(httptest.NewRecorder(), asStaff(patch, f.a.ID, f.adminA))); got != http.StatusInternalServerError {
		t.Errorf("updateEnquiry status = %d", got)
	}
}

// Counting leads runs as a second query after listing them, so it is its own
// failure branch. The column is renamed and put back, because the whole package
// shares one database.
func TestListEnquiriesFailsWhenCountsFail(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	testsupport.NewEnquiry(t, f.db, f.a.SocietyID, nil, "Guest")

	if _, err := f.db.Exec(ctx, `ALTER TABLE enquiries RENAME COLUMN status TO status_hidden`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.db.Exec(context.Background(),
			`ALTER TABLE enquiries RENAME COLUMN status_hidden TO status`); err != nil {
			t.Fatalf("could not restore the schema for the other tests: %v", err)
		}
	})

	if _, err := f.store.Counts(ctx, f.a.SocietyID); err == nil {
		t.Fatal("Counts should fail when the column is gone")
	}

	list := httptest.NewRequest(http.MethodGet, "/x", nil)
	list.SetPathValue("societyId", f.a.SocietyID.String())
	if got := status(t, f.handler.listEnquiries(httptest.NewRecorder(),
		asStaff(list, f.a.ID, f.adminA))); got != http.StatusInternalServerError {
		t.Errorf("listEnquiries status = %d, want 500", got)
	}
}
