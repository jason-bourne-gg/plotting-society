package maintenance

import (
	"context"
	"testing"

	"github.com/jason-bourne-gg/plotting-society/internal/domain"
	"github.com/jason-bourne-gg/plotting-society/internal/testsupport"
)

// Only a sold plot can be billed — nobody owns an available one. Counting
// every plot made the admin page claim 823 sold plots against 515 bills and
// invent 308 invoices that were supposedly outstanding.
func TestListCountsOnlySoldPlots(t *testing.T) {
	db := testsupport.DB(t, "maintenance")
	store := NewStore(db)
	ctx := context.Background()

	b := testsupport.NewBuilder(t, db, "Alpha")
	admin := testsupport.NewUser(t, db, &b.ID, domain.RoleBuilderAdmin, "a@alpha.in", "")
	owner := testsupport.NewUser(t, db, nil, domain.RoleOwner, "o@example.in", "")

	if _, err := db.Exec(ctx,
		`INSERT INTO maintenance_rates (society_id, sector, rate_per_sqft) VALUES ($1, NULL, 11.04)`,
		b.SocietyID); err != nil {
		t.Fatal(err)
	}

	// Three sold, two not. Only the sold ones are billable.
	sold := []string{"1", "2", "3"}
	for _, no := range sold {
		id := testsupport.NewPlot(t, db, b.SocietyID, no, "sold")
		testsupport.AssignPlot(t, db, id, owner)
	}
	testsupport.NewPlot(t, db, b.SocietyID, "4", "available")
	testsupport.NewPlot(t, db, b.SocietyID, "5", "booked")

	rates, _, err := store.List(ctx, b.SocietyID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rates) != 1 {
		t.Fatalf("got %d sectors, want 1", len(rates))
	}
	if rates[0].PlotCount != 3 {
		t.Errorf("PlotCount = %d, want 3 — available and booked plots are not billable", rates[0].PlotCount)
	}
	if rates[0].BilledCount != 0 {
		t.Errorf("BilledCount = %d, want 0", rates[0].BilledCount)
	}

	raised, total, err := store.GenerateBills(ctx, b.SocietyID)
	if err != nil {
		t.Fatalf("GenerateBills: %v", err)
	}
	if raised != 3 {
		t.Errorf("raised %d bills, want 3 — one per sold plot", raised)
	}
	if total <= 0 {
		t.Errorf("total = %v", total)
	}

	rates, _, err = store.List(ctx, b.SocietyID)
	if err != nil {
		t.Fatal(err)
	}
	if rates[0].BilledCount != 3 || rates[0].PlotCount != 3 {
		t.Errorf("after billing: %d/%d, want 3/3", rates[0].BilledCount, rates[0].PlotCount)
	}

	// Re-running must not double-bill.
	again, _, err := store.GenerateBills(ctx, b.SocietyID)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Errorf("re-running raised %d more bills; it must be idempotent", again)
	}
	_ = admin
}

// The reference quote is how the site office sanity-checks a rate.
func TestReferenceAmount(t *testing.T) {
	if got := amountFor(ReferenceAreaSqft, 11.04); got != 17001.60 {
		t.Errorf("1540 sq ft at Rs 11.04 = %v, want 17001.60", got)
	}
	if got := amountFor(1130.22, 12.50); got != 14127.75 {
		t.Errorf("amountFor = %v, want 14127.75", got)
	}
}
