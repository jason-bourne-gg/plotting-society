package testsupport

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
)

// Builder is a builder plus one society, which is the unit almost every test
// needs: the tenancy checks are all "does this record belong to that builder".
type Builder struct {
	ID        uuid.UUID
	SocietyID uuid.UUID
	Slug      string
}

var seq int

// NewBuilder inserts a builder and a society under it.
func NewBuilder(t *testing.T, db *database.DB, name string) Builder {
	t.Helper()
	seq++
	slug := fmt.Sprintf("%s-%d", sanitise(name), seq)

	var b Builder
	b.Slug = slug
	ctx := context.Background()

	if err := db.QueryRow(ctx,
		`INSERT INTO builders (name, slug, phone, email) VALUES ($1,$2,'+91 90000 00000','x@y.in') RETURNING id`,
		name, slug).Scan(&b.ID); err != nil {
		t.Fatalf("insert builder: %v", err)
	}
	if err := db.QueryRow(ctx,
		`INSERT INTO societies (builder_id, name, slug, city, public_listing)
		 VALUES ($1,$2,$3,'Nagpur',true) RETURNING id`,
		b.ID, name+" Society", slug).Scan(&b.SocietyID); err != nil {
		t.Fatalf("insert society: %v", err)
	}
	return b
}

// NewUser inserts a user. Pass a nil builder for an owner.
func NewUser(t *testing.T, db *database.DB, builderID *uuid.UUID, role domain.Role, email, passwordHash string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := db.QueryRow(context.Background(),
		`INSERT INTO users (builder_id, email, name, role, password_hash)
		 VALUES ($1,$2,$3,$4,NULLIF($5,'')) RETURNING id`,
		builderID, email, email, role, passwordHash).Scan(&id); err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	return id
}

// NewPlot inserts a plot in the given society.
func NewPlot(t *testing.T, db *database.DB, societyID uuid.UUID, plotNo, status string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := db.QueryRow(context.Background(),
		`INSERT INTO plots (society_id, plot_no, phase, area_sqft, facing, status, price, map_shape)
		 VALUES ($1,$2,'Sector 01',1291.68,'East',$3,3400000,
		         '{"points":[[0,0],[80,0],[80,60],[0,60]],"sector":1}'::jsonb)
		 RETURNING id`,
		societyID, plotNo, status).Scan(&id); err != nil {
		t.Fatalf("insert plot %s: %v", plotNo, err)
	}
	return id
}

// AssignPlot makes a user the owner of a plot.
func AssignPlot(t *testing.T, db *database.DB, plotID, ownerID uuid.UUID) {
	t.Helper()
	if _, err := db.Exec(context.Background(),
		`UPDATE plots SET owner_id = $1, status = 'sold' WHERE id = $2`, ownerID, plotID); err != nil {
		t.Fatalf("assign plot: %v", err)
	}
}

// NewQuery inserts an owner query.
func NewQuery(t *testing.T, db *database.DB, societyID, plotID, raisedBy uuid.UUID, category, subject string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := db.QueryRow(context.Background(),
		`INSERT INTO queries (society_id, plot_id, raised_by, category, subject, sla_due_at)
		 VALUES ($1,$2,$3,$4,$5, now() + interval '7 days') RETURNING id`,
		societyID, plotID, raisedBy, category, subject).Scan(&id); err != nil {
		t.Fatalf("insert query: %v", err)
	}
	return id
}

// NewFundEntry inserts one ledger row.
func NewFundEntry(t *testing.T, db *database.DB, societyID, createdBy uuid.UUID, head string, credit, debit float64) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := db.QueryRow(context.Background(),
		`INSERT INTO fund_entries (society_id, entry_date, head, description, credit, debit, created_by)
		 VALUES ($1, CURRENT_DATE, $2, 'fixture', $3, $4, $5) RETURNING id`,
		societyID, head, credit, debit, createdBy).Scan(&id); err != nil {
		t.Fatalf("insert fund entry: %v", err)
	}
	return id
}

// NewUpdate inserts a site update.
func NewUpdate(t *testing.T, db *database.DB, societyID, author uuid.UUID, title string, published bool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := db.QueryRow(context.Background(),
		`INSERT INTO site_updates (society_id, title, body, created_by, published_at)
		 VALUES ($1,$2,'body',$3, CASE WHEN $4 THEN now() ELSE NULL END) RETURNING id`,
		societyID, title, author, published).Scan(&id); err != nil {
		t.Fatalf("insert site update: %v", err)
	}
	return id
}

// NewEnquiry inserts a guest lead.
func NewEnquiry(t *testing.T, db *database.DB, societyID uuid.UUID, plotID *uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := db.QueryRow(context.Background(),
		`INSERT INTO enquiries (society_id, plot_id, name, phone, message, source)
		 VALUES ($1,$2,$3,'9822000000','hello','layout_map') RETURNING id`,
		societyID, plotID, name).Scan(&id); err != nil {
		t.Fatalf("insert enquiry: %v", err)
	}
	return id
}
