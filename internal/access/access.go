// Package access answers one question: may this caller act on this record?
//
// Every staff-only route takes an id straight from the URL. `RequireStaff`
// proves the caller is *a* builder; it does not prove they are *this* builder.
// Without the checks here, a staff account at one builder could write ledger
// entries into another builder's society, or attach an owner to a plot that was
// never theirs, simply by changing the id in the path.
//
// The signatures take role and builder id rather than an auth.Identity so that
// package auth can use this too without an import cycle.
package access

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/httpx"
)

type Guard struct{ db *database.DB }

func NewGuard(db *database.DB) *Guard { return &Guard{db: db} }

// denied is deliberately a 404, not a 403: telling a caller "that exists but is
// not yours" turns an id into an oracle for enumerating other builders' data.
func denied() error {
	return httpx.NotFound("That record does not exist.")
}

// allowAll reports whether the role skips tenancy checks entirely.
func allowAll(role domain.Role) bool { return role == domain.RoleSuperAdmin }

func (g *Guard) check(ctx context.Context, role domain.Role, builderID *uuid.UUID, query string, id uuid.UUID) error {
	if allowAll(role) {
		// A super admin still must not act on an id that does not exist.
		var exists bool
		err := g.db.QueryRow(ctx, query, id, uuid.Nil).Scan(&exists)
		if errors.Is(err, pgx.ErrNoRows) {
			return denied()
		}
		if err != nil {
			return httpx.Internal(err)
		}
		return nil
	}
	if builderID == nil {
		return denied()
	}

	var owned bool
	err := g.db.QueryRow(ctx, query, id, *builderID).Scan(&owned)
	if errors.Is(err, pgx.ErrNoRows) {
		return denied()
	}
	if err != nil {
		return httpx.Internal(err)
	}
	if !owned {
		return denied()
	}
	return nil
}

// Society allows the caller to act on a society their builder owns.
func (g *Guard) Society(ctx context.Context, role domain.Role, builderID *uuid.UUID, societyID uuid.UUID) error {
	return g.check(ctx, role, builderID,
		`SELECT builder_id = $2 FROM societies WHERE id = $1`, societyID)
}

// Plot allows the caller to act on a plot inside a society their builder owns.
func (g *Guard) Plot(ctx context.Context, role domain.Role, builderID *uuid.UUID, plotID uuid.UUID) error {
	return g.check(ctx, role, builderID,
		`SELECT s.builder_id = $2 FROM plots p JOIN societies s ON s.id = p.society_id WHERE p.id = $1`,
		plotID)
}

// Query allows the caller to act on an owner query in their builder's society.
func (g *Guard) Query(ctx context.Context, role domain.Role, builderID *uuid.UUID, queryID uuid.UUID) error {
	return g.check(ctx, role, builderID,
		`SELECT s.builder_id = $2 FROM queries q JOIN societies s ON s.id = q.society_id WHERE q.id = $1`,
		queryID)
}

// FundEntry allows the caller to reverse an entry in their builder's ledger.
func (g *Guard) FundEntry(ctx context.Context, role domain.Role, builderID *uuid.UUID, entryID uuid.UUID) error {
	return g.check(ctx, role, builderID,
		`SELECT s.builder_id = $2 FROM fund_entries f JOIN societies s ON s.id = f.society_id WHERE f.id = $1`,
		entryID)
}

// Update allows the caller to publish a progress post in their builder's society.
func (g *Guard) Update(ctx context.Context, role domain.Role, builderID *uuid.UUID, postID uuid.UUID) error {
	return g.check(ctx, role, builderID,
		`SELECT s.builder_id = $2 FROM site_updates u JOIN societies s ON s.id = u.society_id WHERE u.id = $1`,
		postID)
}

// Enquiry allows the caller to work a lead belonging to their builder.
func (g *Guard) Enquiry(ctx context.Context, role domain.Role, builderID *uuid.UUID, enquiryID uuid.UUID) error {
	return g.check(ctx, role, builderID,
		`SELECT s.builder_id = $2 FROM enquiries e JOIN societies s ON s.id = e.society_id WHERE e.id = $1`,
		enquiryID)
}
