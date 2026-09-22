package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

func TestBehaviourWhenTheDatabaseIsDown(t *testing.T) {
	f := newFixture(t)
	postID := testsupport.NewUpdate(t, f.db, f.a.SocietyID, f.adminA, "Draft", false)
	societyID := f.a.SocietyID
	admin := ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin)
	f.db.Close()

	ctx := context.Background()

	if _, err := f.store.List(ctx, societyID, true, 10); err == nil {
		t.Error("List should fail")
	}
	if _, err := f.store.Create(ctx, societyID, f.adminA, NewPost{Title: "t"}); err == nil {
		t.Error("Create should fail")
	}
	if err := f.store.Publish(ctx, postID); err == nil {
		t.Error("Publish should fail")
	}

	list := httptest.NewRequest(http.MethodGet, "/x", nil)
	list.SetPathValue("societyId", societyID.String())
	if got := apiStatus(t, f.h.list(httptest.NewRecorder(), list)); got != http.StatusInternalServerError {
		t.Errorf("list status = %d", got)
	}

	create := as(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"title":"t"}`)), admin)
	create.SetPathValue("societyId", societyID.String())
	if got := apiStatus(t, f.h.create(httptest.NewRecorder(), create)); got != http.StatusInternalServerError {
		t.Errorf("create status = %d", got)
	}

	publish := as(httptest.NewRequest(http.MethodPost, "/x", nil), admin)
	publish.SetPathValue("postId", postID.String())
	if got := apiStatus(t, f.h.publish(httptest.NewRecorder(), publish)); got != http.StatusInternalServerError {
		t.Errorf("publish status = %d", got)
	}
}

// A post with media runs a second query to attach them, which is its own
// failure branch.
func TestListFailsWhenMediaCannotBeRead(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.store.Create(ctx, f.a.SocietyID, f.adminA, NewPost{
		Title: "with media", Publish: true,
		Media: []Media{{URL: "https://pub.r2.dev/a.jpg"}},
	}); err != nil {
		t.Fatal(err)
	}

	// Hiding the media table leaves the posts query working and the follow-up
	// failing, which is exactly the branch under test. It is renamed rather than
	// dropped, and put back afterwards, because the whole package shares one
	// database.
	if _, err := f.db.Exec(ctx, `ALTER TABLE site_update_media RENAME TO site_update_media_hidden`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.db.Exec(context.Background(),
			`ALTER TABLE site_update_media_hidden RENAME TO site_update_media`); err != nil {
			t.Fatalf("could not restore the schema for the other tests: %v", err)
		}
	})

	if _, err := f.store.List(ctx, f.a.SocietyID, true, 10); err == nil {
		t.Fatal("List should fail when the media query fails")
	}
}
