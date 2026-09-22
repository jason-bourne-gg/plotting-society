package access

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

// Every check in this package exists to stop one thing: staff at builder A
// acting on builder B's records by editing an id in the URL. Each case is run
// three ways — the owning builder (allowed), a different builder (denied), and
// a super admin (allowed) — because a check that is merely present but wired to
// the wrong column would still pass a single happy-path test.

type world struct {
	db   *database.DB
	g    *Guard
	a, b testsupport.Builder
	// ids that belong to builder a
	plotID, queryID, fundID, updateID, enquiryID uuid.UUID
}

func setup(t *testing.T) world {
	t.Helper()
	db := testsupport.DB(t, "access")
	w := world{db: db, g: NewGuard(db)}

	w.a = testsupport.NewBuilder(t, db, "Alpha")
	w.b = testsupport.NewBuilder(t, db, "Beta")

	adminA := testsupport.NewUser(t, db, &w.a.ID, domain.RoleBuilderAdmin, "a@alpha.in", "")
	ownerA := testsupport.NewUser(t, db, nil, domain.RoleOwner, "o@alpha.in", "")

	w.plotID = testsupport.NewPlot(t, db, w.a.SocietyID, "1", "sold")
	testsupport.AssignPlot(t, db, w.plotID, ownerA)
	w.queryID = testsupport.NewQuery(t, db, w.a.SocietyID, w.plotID, ownerA, "documents_legal", "s")
	w.fundID = testsupport.NewFundEntry(t, db, w.a.SocietyID, adminA, "roads", 0, 1000)
	w.updateID = testsupport.NewUpdate(t, db, w.a.SocietyID, adminA, "post", true)
	w.enquiryID = testsupport.NewEnquiry(t, db, w.a.SocietyID, &w.plotID, "Guest")
	return w
}

func statusOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return http.StatusOK
	}
	var apiErr *httpx.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected an httpx.Error, got %T: %v", err, err)
	}
	return apiErr.Status
}

func TestGuardScopesEveryRecordToItsBuilder(t *testing.T) {
	w := setup(t)
	ctx := context.Background()

	checks := map[string]func(role domain.Role, builderID *uuid.UUID) error{
		"Society": func(r domain.Role, b *uuid.UUID) error {
			return w.g.Society(ctx, r, b, w.a.SocietyID)
		},
		"Plot":     func(r domain.Role, b *uuid.UUID) error { return w.g.Plot(ctx, r, b, w.plotID) },
		"Query":    func(r domain.Role, b *uuid.UUID) error { return w.g.Query(ctx, r, b, w.queryID) },
		"FundEntry": func(r domain.Role, b *uuid.UUID) error { return w.g.FundEntry(ctx, r, b, w.fundID) },
		"Update":   func(r domain.Role, b *uuid.UUID) error { return w.g.Update(ctx, r, b, w.updateID) },
		"Enquiry":  func(r domain.Role, b *uuid.UUID) error { return w.g.Enquiry(ctx, r, b, w.enquiryID) },
	}

	for name, check := range checks {
		t.Run(name+"/owning builder allowed", func(t *testing.T) {
			if err := check(domain.RoleBuilderAdmin, &w.a.ID); err != nil {
				t.Errorf("the owning builder was denied: %v", err)
			}
		})

		t.Run(name+"/other builder denied", func(t *testing.T) {
			err := check(domain.RoleBuilderAdmin, &w.b.ID)
			if err == nil {
				t.Fatal("a different builder was allowed through — this is the bug the guard exists to prevent")
			}
			// 404 rather than 403: a 403 confirms the record exists, which
			// turns an id into an oracle for enumerating other builders.
			if got := statusOf(t, err); got != http.StatusNotFound {
				t.Errorf("status = %d, want 404 so ids cannot be probed", got)
			}
		})

		t.Run(name+"/super admin allowed", func(t *testing.T) {
			if err := check(domain.RoleSuperAdmin, nil); err != nil {
				t.Errorf("a super admin was denied: %v", err)
			}
		})

		t.Run(name+"/staff with no builder denied", func(t *testing.T) {
			if err := check(domain.RoleBuilderStaff, nil); err == nil {
				t.Error("staff with a nil builder id must be denied")
			}
		})
	}
}

func TestGuardRejectsUnknownIDs(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	missing := uuid.New()

	checks := map[string]error{
		"Society":   w.g.Society(ctx, domain.RoleBuilderAdmin, &w.a.ID, missing),
		"Plot":      w.g.Plot(ctx, domain.RoleBuilderAdmin, &w.a.ID, missing),
		"Query":     w.g.Query(ctx, domain.RoleBuilderAdmin, &w.a.ID, missing),
		"FundEntry": w.g.FundEntry(ctx, domain.RoleBuilderAdmin, &w.a.ID, missing),
		"Update":    w.g.Update(ctx, domain.RoleBuilderAdmin, &w.a.ID, missing),
		"Enquiry":   w.g.Enquiry(ctx, domain.RoleBuilderAdmin, &w.a.ID, missing),
	}
	for name, err := range checks {
		if got := statusOf(t, err); got != http.StatusNotFound {
			t.Errorf("%s with an unknown id: status = %d, want 404", name, got)
		}
	}
}

// A super admin bypasses tenancy but must still not act on a row that is gone.
func TestSuperAdminStillNeedsTheRecordToExist(t *testing.T) {
	w := setup(t)
	if err := w.g.Society(context.Background(), domain.RoleSuperAdmin, nil, uuid.New()); err == nil {
		t.Fatal("a super admin was allowed to act on a society that does not exist")
	}
}

// A database failure must surface as a 500, not as a silent allow. Closing the
// pool is the cheapest honest way to exercise that branch.
func TestGuardReportsDatabaseFailures(t *testing.T) {
	db := testsupport.DB(t, "access")
	b := testsupport.NewBuilder(t, db, "Gamma")
	g := NewGuard(db)
	db.Close()

	ctx := context.Background()

	if got := statusOf(t, g.Society(ctx, domain.RoleBuilderAdmin, &b.ID, b.SocietyID)); got != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when the database is unreachable", got)
	}
	if got := statusOf(t, g.Society(ctx, domain.RoleSuperAdmin, nil, b.SocietyID)); got != http.StatusInternalServerError {
		t.Errorf("super admin path: status = %d, want 500", got)
	}
}
