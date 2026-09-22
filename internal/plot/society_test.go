package plot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

// Returning every society to any signed-in user would leak every builder's
// project list, so the listing is scoped three ways.
func TestSocietiesIsScopedToTheCaller(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	t.Run("staff see only their builder", func(t *testing.T) {
		list, err := f.store.Societies(ctx, domain.RoleBuilderAdmin, &f.a.ID, f.adminA)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ID != f.a.SocietyID {
			t.Fatalf("staff saw %d societies: %+v", len(list), list)
		}
	})

	t.Run("an owner sees the societies they hold a plot in", func(t *testing.T) {
		list, err := f.store.Societies(ctx, domain.RoleOwner, nil, f.owner)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ID != f.a.SocietyID {
			t.Fatalf("the owner saw %d societies", len(list))
		}
	})

	t.Run("an owner with no plot sees nothing", func(t *testing.T) {
		stranger := testsupport.NewUser(t, f.db, nil, domain.RoleOwner, "nobody@example.in", "")
		list, err := f.store.Societies(ctx, domain.RoleOwner, nil, stranger)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 0 {
			t.Fatalf("a user with no plot saw %d societies", len(list))
		}
	})

	t.Run("a super admin sees everything", func(t *testing.T) {
		list, err := f.store.Societies(ctx, domain.RoleSuperAdmin, nil, f.adminA)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 {
			t.Fatalf("the super admin saw %d societies, want 2", len(list))
		}
	})
}


func TestSocietyHandlers(t *testing.T) {
	f := newFixture(t)

	t.Run("list", func(t *testing.T) {
		r := as(httptest.NewRequest(http.MethodGet, "/x", nil),
			ident(f.adminA, &f.a.ID, domain.RoleBuilderAdmin))
		rec := httptest.NewRecorder()
		if err := f.h.listSocieties(rec, r); err != nil {
			t.Fatal(err)
		}
		list, _ := jsonBody(t, rec)["societies"].([]any)
		if len(list) != 1 {
			t.Errorf("got %d societies", len(list))
		}
	})

}

func TestPlotRoutesAreRegistered(t *testing.T) {
	f := newFixture(t)
	mux := http.NewServeMux()
	authn := testAuthenticator()
	f.h.Routes(mux, authn)
	f.h.SocietyRoutes(mux, authn)

	// The map is open to guests.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/societies/"+f.a.SocietyID.String()+"/plots", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("guest map status = %d, want 200", rec.Code)
	}

	// Plot detail is not.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/plots/"+f.sold.String(), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("plot detail status = %d, want 401", rec.Code)
	}
}

// Registering the full route set must not panic. Go's ServeMux rejects
// overlapping patterns at registration time, so a conflict here is a crash at
// boot rather than a bad response later — exactly the sort of thing that only
// shows up when something actually mounts the routes.
func TestRouteRegistrationDoesNotConflict(t *testing.T) {
	f := newFixture(t)
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("registering routes panicked, so the server would not start: %v", rec)
		}
	}()

	mux := http.NewServeMux()
	authn := testAuthenticator()
	f.h.Routes(mux, authn)
	f.h.SocietyRoutes(mux, authn)
}
