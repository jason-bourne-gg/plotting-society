// Command seed loads the Sandesh Nagari 7 layout: Shiv Rudra Group's 823-plot,
// 58-acre NMRDA-sanctioned project at Rui & Banwadi on the Wardha Road–MIHAN
// corridor, Nagpur.
//
// Plot numbers, sector ranges and plot areas come from the sanctioned layout in
// the project brochure. Ownership, dues, ledger and queries are invented — they
// have to be, since that data lives in the builder's own records.
//
// Safe to re-run: it refuses a database that already holds a society unless
// -force is passed.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jason-bourne-gg/plotting-society/internal/auth"
	"github.com/jason-bourne-gg/plotting-society/internal/database"
)

// sector mirrors the brochure's four sanctioned sectors.
type sector struct {
	number  int
	name    string
	first   int
	last    int
	cols    int
	colour  string
	originY int
}

var sectors = []sector{
	{1, "Sector 01", 1, 302, 22, "#E8913A", 60},
	{2, "Sector 02", 303, 364, 16, "#4FA3DC", 780},
	{3, "Sector 03", 365, 517, 18, "#57A55B", 1060},
	{4, "Sector 04", 518, 823, 22, "#8B7EC8", 1640},
}

// The site office quotes maintenance against a reference plot: 1,540 sq ft
// costs Rs 17,000 one time. Everything else is unitary from that — Rs 11.04 per
// square foot — so a 1,130 sq ft plot pays Rs 12,475 and a 4,035 sq ft plot
// pays Rs 44,546.
//
// Sector 01 fronts the 15 M spine road and carries more lighting and sweeping,
// so it sits above the baseline; Sector 04 is still being developed and sits
// below it. These are the rates the admin can change per sector.
const (
	referenceAreaSqft   = 1540.0
	referenceAmount     = 17000.0
	baselineRatePerSqft = referenceAmount / referenceAreaSqft // Rs 11.04
)

var sectorRates = map[string]float64{
	"Sector 01": 12.50,
	"Sector 02": 11.04,
	"Sector 03": 11.04,
	"Sector 04": 9.75,
}

// plotAreas are the distinct "remaining plot area" figures that repeat across
// the brochure's area statements, in square feet.
var plotAreas = []float64{
	1130.22, 1205.57, 1210.95, 1251.32, 1291.68, 1356.26, 1372.41, 1412.78,
	1450.71, 1453.14, 1463.90, 1533.87, 1571.80, 1596.84, 1664.88, 1736.80,
	1910.84, 2069.64, 2282.84, 2500.01, 2797.23, 3024.26, 3422.31,
}

func main() {
	force := flag.Bool("force", false, "seed even if the database already has a society")
	password := flag.String("password", "sandesh-demo-2026", "password for the seeded logins")
	flag.Parse()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := database.Connect(ctx, url)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	var existing int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM societies`).Scan(&existing); err != nil {
		log.Fatalf("count societies: %v", err)
	}
	if existing > 0 && !*force {
		log.Fatalf("database already holds %d society(ies); pass -force to seed anyway", existing)
	}
	if *force {
		if _, err := db.Exec(ctx, `TRUNCATE builders CASCADE`); err != nil {
			log.Fatalf("truncate: %v", err)
		}
	}

	if err := seed(ctx, db, *password); err != nil {
		log.Fatalf("seed: %v", err)
	}
}

func seed(ctx context.Context, db *database.DB, password string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var builderID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO builders (name, slug, phone, email)
		VALUES ('Shiv Rudra Group', 'shiv-rudra-group', '+91 91560 00007', 'sales@shivrudragroup.in')
		RETURNING id`).Scan(&builderID); err != nil {
		return fmt.Errorf("builder: %w", err)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	var adminID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (builder_id, email, name, role, password_hash, phone, city)
		VALUES ($1, 'office@shivrudragroup.in', 'Sandesh Nagari Site Office', 'builder_admin', $2,
		        '+91 91560 00007', 'Nagpur')
		RETURNING id`, builderID, hash).Scan(&adminID); err != nil {
		return fmt.Errorf("admin: %w", err)
	}

	var staffID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (builder_id, email, name, role, password_hash, phone, city)
		VALUES ($1, 'sitedesk@shivrudragroup.in', 'Prashant — Site Desk', 'builder_staff', $2,
		        '+91 91560 00012', 'Nagpur')
		RETURNING id`, builderID, hash).Scan(&staffID); err != nil {
		return fmt.Errorf("staff: %w", err)
	}

	// Marketing copy for the guest view, taken from the project brochure.
	highlights := `[
	  {"label":"823 Plots","detail":"Across four sanctioned sectors"},
	  {"label":"58 Acres","detail":"NMRDA sanctioned layout"},
	  {"label":"RL Plots","detail":"Full development, ready for construction"},
	  {"label":"10 minutes","detail":"To AIIMS, IIM Nagpur and the MIHAN SEZ"},
	  {"label":"5 minutes","detail":"To Wardha Road / NH44"},
	  {"label":"2 minutes","detail":"To the Samruddhi Mahamarg extension"}
	]`
	// Travel times as printed in the brochure.
	landmarks := `[
	  {"name":"Wardha Road / NH44","minutes":5},
	  {"name":"Samruddhi Mahamarg extension","minutes":2},
	  {"name":"AIIMS Nagpur","minutes":10},
	  {"name":"IIM Nagpur","minutes":10},
	  {"name":"MIHAN SEZ","minutes":10},
	  {"name":"Nagpur Airport","minutes":15},
	  {"name":"Suretech Hospital","minutes":10},
	  {"name":"Achiever School","minutes":4},
	  {"name":"VCA Cricket Stadium","minutes":15},
	  {"name":"Le Meridien Hotel","minutes":10}
	]`

	amenities := `[
	  "Club House","Swimming Pool","Open Gym","Multipurpose Court","Amphitheatre",
	  "Badminton / Volleyball / Pickleball Court","Lawn","Cricket Pitch","Play Court",
	  "Walking Path","Cycling Track","Fitness Park","Floral Park","Reading Zone",
	  "Fruit Farm","Vegetable Garden","Reflexology Path","Herb Garden","Aromatic Zone",
	  "Relaxation Zone","Kids Play Area","Pet Park","Sandpit Area","Calisthenics Zone",
	  "Butterfly Garden","Yoga & Meditation Park","Entrance Gate","Cement Roads",
	  "Street Lights","STP","Drainage Line","Storm Line","Electrification"
	]`

	var societyID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO societies (builder_id, name, slug, city, address, rera_number,
		                       layout_width, layout_height, tagline, highlights, amenities,
		                       contact_phone, contact_email, public_listing,
		                       map_label, landmarks)
		VALUES ($1, 'Sandesh Nagari 7', 'sandesh-nagari-7', 'Nagpur',
		        'Rui & Banwadi, Wardha Road – MIHAN Corridor, Nagpur. Office: G1 Lalita Apartment, Behind Domino''s, Somalwada, Nagpur 440025',
		        'PP1190002601297', 2400, 2200,
		        'The address that Nagpur is heading towards.',
		        $2::jsonb, $3::jsonb, '+91 91560 00007', 'sales@shivrudragroup.in', true,
		        'Sandesh Nagari 7, Rui & Banwadi', $4::jsonb)
		RETURNING id`, builderID, highlights, amenities, landmarks).Scan(&societyID); err != nil {
		return fmt.Errorf("society: %w", err)
	}

	// The society-wide fallback, then the per-sector overrides the site office
	// would have set.
	if _, err := tx.Exec(ctx, `
		INSERT INTO maintenance_rates (society_id, sector, rate_per_sqft, updated_by)
		VALUES ($1, NULL, $2, $3)`,
		societyID, math.Round(baselineRatePerSqft*100)/100, adminID); err != nil {
		return fmt.Errorf("default rate: %w", err)
	}
	for sector, rate := range sectorRates {
		if _, err := tx.Exec(ctx, `
			INSERT INTO maintenance_rates (society_id, sector, rate_per_sqft, updated_by)
			VALUES ($1, $2, $3, $4)`, societyID, sector, rate, adminID); err != nil {
			return fmt.Errorf("rate for %s: %w", sector, err)
		}
	}

	// Deterministic, so a demo shown twice looks the same twice.
	rng := rand.New(rand.NewSource(7))

	// Roughly 61% sold, which is what a project part-way through selling looks
	// like, and leaves plenty of "available" green on the map to point at.
	pickStatus := func() string {
		switch n := rng.Intn(100); {
		case n < 61:
			return "sold"
		case n < 74:
			return "booked"
		case n < 79:
			return "on_hold"
		default:
			return "available"
		}
	}

	const cellW, cellH, gap = 84, 64, 6

	soldPlots := make([]uuid.UUID, 0, 512)
	plotIDByNo := map[int]uuid.UUID{}

	// Insert in batches rather than one statement per plot.
	//
	// 823 round trips inside a single transaction, across the internet and
	// through a connection pooler, is slow and brittle — it died partway with
	// "unexpected EOF" holding a server connection the whole time. Ids are
	// generated here instead of returned so the batch needs no RETURNING and
	// the caller still knows which plot is which.
	const batchSize = 150
	var (
		args     []any
		values   []string
		pending  int
		rowIndex int
	)

	flush := func() error {
		if pending == 0 {
			return nil
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO plots (id, society_id, plot_no, phase, area_sqft, facing, is_corner, status, price, map_shape)
			 VALUES `+strings.Join(values, ","), args...)
		args, values, pending = args[:0], values[:0], 0
		return err
	}

	for _, sec := range sectors {
		for no := sec.first; no <= sec.last; no++ {
			idx := no - sec.first
			col := idx % sec.cols
			row := idx / sec.cols

			x := 60 + col*(cellW+gap)
			y := sec.originY + row*(cellH+gap)
			shape := fmt.Sprintf(
				`{"points":[[%d,%d],[%d,%d],[%d,%d],[%d,%d]],"sector":%d,"colour":"%s"}`,
				x, y, x+cellW, y, x+cellW, y+cellH, x, y+cellH, sec.number, sec.colour)

			area := plotAreas[rng.Intn(len(plotAreas))]
			price := area * float64(2400+rng.Intn(600))
			status := pickStatus()
			corner := col == 0 || col == sec.cols-1

			id := uuid.New()
			plotIDByNo[no] = id
			if status == "sold" {
				soldPlots = append(soldPlots, id)
			}

			base := rowIndex * 10
			values = append(values, fmt.Sprintf(
				"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d::jsonb)",
				base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10))
			args = append(args, id, societyID, fmt.Sprintf("%d", no), sec.name, area,
				[]string{"East", "West", "North", "South"}[rng.Intn(4)], corner, status, price, shape)
			pending++
			rowIndex++

			if pending >= batchSize {
				if err := flush(); err != nil {
					return fmt.Errorf("insert plots: %w", err)
				}
				rowIndex = 0
			}
		}
	}
	if err := flush(); err != nil {
		return fmt.Errorf("insert plots: %w", err)
	}

	// Two owner logins on real sold plots, so the owner-side views have content.
	owners := []struct {
		email, name, phone, address, city string
		plotNo                            int
	}{
		{"rohit.deshmukh@example.com", "Rohit Deshmukh", "+91 99876 54321",
			"Flat 1204, Marvel Zephyr, Kharadi", "Pune", 147},
		{"anjali.kulkarni@example.com", "Anjali Kulkarni", "+91 98230 11884",
			"Villa 9, Palm Meadows, Whitefield", "Bengaluru", 412},
	}

	var primaryOwnerID, primaryPlotID uuid.UUID
	for i, o := range owners {
		var ownerID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (email, name, role, password_hash, phone, current_address, city, directory_opt_in)
			VALUES ($1,$2,'owner',$3,$4,$5,$6,$7) RETURNING id`,
			o.email, o.name, hash, o.phone, o.address, o.city, i == 0).Scan(&ownerID); err != nil {
			return fmt.Errorf("owner %s: %w", o.email, err)
		}
		plotID := plotIDByNo[o.plotNo]
		if _, err := tx.Exec(ctx,
			`UPDATE plots SET owner_id = $1, status = 'sold' WHERE id = $2`, ownerID, plotID); err != nil {
			return err
		}
		if i == 0 {
			primaryOwnerID, primaryPlotID = ownerID, plotID
		}
	}

	// One-time maintenance, priced per square foot, raised against every plot
	// that has been sold. The bill snapshots the rate and the area it was
	// computed from so an owner can be shown the working and a later rate
	// change cannot rewrite an invoice already issued.
	//
	// About four in five have paid, which is what a real collection looks like
	// and gives the builder's dues view something to chase.
	var (
		billed      int
		billedPaid  int
		collected   float64
		outstanding float64
	)
	rows, err := tx.Query(ctx, `
		SELECT id, area_sqft, phase FROM plots
		 WHERE society_id = $1 AND status = 'sold' AND area_sqft IS NOT NULL
		 ORDER BY plot_no`, societyID)
	if err != nil {
		return fmt.Errorf("read sold plots: %w", err)
	}
	type bill struct {
		plotID uuid.UUID
		area   float64
		sector string
	}
	var bills []bill
	for rows.Next() {
		var b bill
		if err := rows.Scan(&b.plotID, &b.area, &b.sector); err != nil {
			rows.Close()
			return err
		}
		bills = append(bills, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Batched for the same reason the plots are.
	dueArgs := make([]any, 0, batchSize*8)
	dueVals := make([]string, 0, batchSize)
	dueIdx := 0

	flushDues := func() error {
		if len(dueVals) == 0 {
			return nil
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO maintenance_dues (plot_id, period_label, amount_due, amount_paid,
			                               due_date, paid_on, rate_per_sqft, area_sqft, sector)
			 VALUES `+strings.Join(dueVals, ","), dueArgs...)
		dueArgs, dueVals, dueIdx = dueArgs[:0], dueVals[:0], 0
		return err
	}

	for i, b := range bills {
		rate, ok := sectorRates[b.sector]
		if !ok {
			rate = math.Round(baselineRatePerSqft*100) / 100
		}
		amount := math.Round(b.area*rate*100) / 100

		// Deterministic 80/20 split rather than a random one, so the ledger
		// below always reconciles against what is actually on the plots.
		amountPaid := 0.0
		var paidOn any
		if i%5 != 0 {
			amountPaid = amount
			paidOn = time.Now().AddDate(0, 0, -(30 + rng.Intn(300)))
			collected += amount
			billedPaid++
		} else {
			outstanding += amount
		}

		base := dueIdx * 8
		dueVals = append(dueVals, fmt.Sprintf(
			"($%d,'One-time maintenance',$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8))
		dueArgs = append(dueArgs, b.plotID, amount, amountPaid,
			time.Now().AddDate(0, 0, -400), paidOn, rate, b.area, b.sector)
		dueIdx++
		billed++

		if dueIdx >= batchSize {
			if err := flushDues(); err != nil {
				return fmt.Errorf("insert dues: %w", err)
			}
		}
	}
	if err := flushDues(); err != nil {
		return fmt.Errorf("insert dues: %w", err)
	}

	// The society fund is maintenance money collected from owners — security,
	// water, lighting, upkeep. It deliberately does NOT carry the developer's
	// capital works (roads, the STP, the club house): those are funded from
	// plot sales and would leave the owners' fund tens of lakhs in deficit,
	// which is both wrong and an alarming thing to put in front of a builder.
	ledger := []struct {
		days   int
		head   string
		desc   string
		credit float64
		debit  float64
	}{
		{150, "security", "Security agency, 6 guards — Jan to Mar", 0, 792000},
		{131, "lighting", "Street light energy charges and fittings — Q1", 0, 214000},
		{118, "water", "Water tanker supply and borewell power — Q1", 0, 186000},
		{104, "landscaping", "Open Space-7 lawn upkeep and plantation", 0, 262000},
		{84, "security", "Security agency, 6 guards — Apr to Jun", 0, 792000},
		{73, "water", "Borewell servicing and pump repair, Sector 02", 0, 340000},
		{61, "lighting", "Street light energy charges — Q2", 0, 228000},
		{50, "housekeeping", "Road sweeping and garbage clearance — Q2", 0, 174000},
		{38, "admin", "Accounting, RERA filing and society printing", 0, 96000},
		{12, "security", "Security agency, 6 guards — Jul to Sep", 0, 792000},
		{6, "landscaping", "Monsoon replanting along the 18.0 M D.P. road", 0, 148000},
	}
	// One credit for what the one-time maintenance actually brought in, so the
	// ledger reconciles against the dues rather than being a separate fiction.
	if _, err := tx.Exec(ctx, `
		INSERT INTO fund_entries (society_id, entry_date, head, description, credit, debit, created_by)
		VALUES ($1, CURRENT_DATE - 160, 'collections', $2, $3, 0, $4)`,
		societyID,
		fmt.Sprintf("One-time maintenance collected — %d of %d plots", billedPaid, billed),
		collected, adminID); err != nil {
		return fmt.Errorf("collections entry: %w", err)
	}

	for _, e := range ledger {
		if _, err := tx.Exec(ctx, `
			INSERT INTO fund_entries (society_id, entry_date, head, description, credit, debit, created_by)
			VALUES ($1, CURRENT_DATE - $2::int, $3, $4, $5, $6, $7)`,
			societyID, e.days, e.head, e.desc, e.credit, e.debit, adminID); err != nil {
			return fmt.Errorf("fund: %w", err)
		}
	}

	posts := []struct {
		days  int
		title string
		body  string
		phase string
	}{
		{60, "Sector 01 cement roads complete",
			"All 9.0 M and 12.0 M internal roads in Sector 01 are cast and cured. The 15.0 M spine road connecting to the existing road is next.", "Sector 01"},
		{46, "Entrance gate structure up",
			"The main entrance gate columns are cast and the arch is in place. Cladding and signage follow once the approach road is tarred.", ""},
		{34, "STP commissioned in Sector 03",
			"The sewage treatment plant next to Open Space-11 is commissioned and under trial run. Connections to Sector 03 plots are being laid this month.", "Sector 03"},
		{21, "74 street lights energised",
			"Street lighting across Sector 01 and the Sector 02 approach is now live. Sector 04 poles are scheduled after the drainage line closes.", "Sector 01"},
		{9, "Club house plinth cast",
			"Foundation and plinth for the club house near Open Space-3 are complete. Superstructure work begins after the monsoon.", ""},
		{2, "Plantation along the 18.0 M D.P. road",
			"320 saplings planted along the Sector 03 D.P. road frontage as part of the landscaping plan.", "Sector 03"},
	}
	for _, p := range posts {
		if _, err := tx.Exec(ctx, `
			INSERT INTO site_updates (society_id, title, body, phase, published_at, created_by)
			VALUES ($1,$2,$3,NULLIF($4,''), now() - ($5::int || ' days')::interval, $6)`,
			societyID, p.title, p.body, p.phase, p.days, adminID); err != nil {
			return fmt.Errorf("update post: %w", err)
		}
	}

	// One breached query on purpose, so the builder's inbox demonstrates what a
	// missed SLA looks like rather than only the happy path.
	queries := []struct {
		category string
		subject  string
		body     string
		daysAgo  int
		slaDays  int
		status   string
	}{
		{"documents_legal", "Sale deed scan not received for Plot 147",
			"Registration was completed in March at the Nagpur sub-registrar office. I still do not have a scanned copy of the sale deed for my records.",
			12, 7, "open"},
		{"infrastructure", "Street light out near Plot 147",
			"The pole on the 12.0 M road outside my plot has been off for about two weeks. Neighbouring plots on the same stretch are lit.",
			5, 10, "in_progress"},
		{"site_visit", "Video walkthrough request before I travel",
			"I am in Bengaluru until December. Could someone send a short video of the plot and the approach road so I can plan the compound wall?",
			2, 3, "open"},
		{"payments_dues", "FY2026-Q4 maintenance invoice",
			"I can see Q4 is outstanding but I have not received an invoice. Please share it so I can transfer the amount.",
			1, 3, "open"},
	}
	for _, q := range queries {
		var queryID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO queries (society_id, plot_id, raised_by, category, subject, status,
			                     created_at, updated_at, sla_due_at)
			VALUES ($1,$2,$3,$4,$5,$6,
			        now() - ($7::int || ' days')::interval,
			        now() - ($7::int || ' days')::interval,
			        now() - ($7::int || ' days')::interval + ($8::int || ' days')::interval)
			RETURNING id`,
			societyID, primaryPlotID, primaryOwnerID, q.category, q.subject, q.status,
			q.daysAgo, q.slaDays).Scan(&queryID); err != nil {
			return fmt.Errorf("query: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO query_messages (query_id, author_id, body, created_at)
			VALUES ($1,$2,$3, now() - ($4::int || ' days')::interval)`,
			queryID, primaryOwnerID, q.body, q.daysAgo); err != nil {
			return fmt.Errorf("query message: %w", err)
		}
		if q.status == "in_progress" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO query_messages (query_id, author_id, body, created_at)
				VALUES ($1,$2,$3, now() - ($4::int || ' days')::interval)`,
				queryID, staffID,
				"Noted. Our electrical contractor is on site Thursday and will check the whole stretch, not just that pole.",
				q.daysAgo-1); err != nil {
				return err
			}
		}
	}

	// Guest enquiries, so the builder's lead inbox has something in it.
	leads := []struct {
		name, phone, email, msg, budget, source, status string
		plotNo                                          int
		daysAgo                                         int
	}{
		{"Sagar Waghmare", "+91 98220 41190", "sagar.w@example.com",
			"Interested in a corner plot in Sector 01. Is 147 or anything near it still open?",
			"40-50 L", "plot_detail", "new", 152, 1},
		{"Meera Iyer", "+91 99604 23117", "",
			"Looking for 1,500 sq ft plus, east facing. Can I visit this Sunday?",
			"50-60 L", "layout_map", "contacted", 0, 3},
		{"Dr. Ajay Bhoyar", "+91 94220 88301", "ajay.bhoyar@example.com",
			"What is the possession timeline for Sector 03? I want to start construction next year.",
			"60 L+", "layout_map", "visit_scheduled", 0, 6},
		{"Nikhil Raut", "+91 70205 33449", "",
			"Please share the brochure and the payment plan.",
			"", "brochure", "new", 0, 9},
	}
	for _, l := range leads {
		var plotID any
		if l.plotNo > 0 {
			plotID = plotIDByNo[l.plotNo]
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO enquiries (society_id, plot_id, name, phone, email, message, budget,
			                       source, status, created_at, updated_at)
			VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,NULLIF($7,''),$8,$9,
			        now() - ($10::int || ' days')::interval,
			        now() - ($10::int || ' days')::interval)`,
			societyID, plotID, l.name, l.phone, l.email, l.msg, l.budget,
			l.source, l.status, l.daysAgo); err != nil {
			return fmt.Errorf("enquiry: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	fmt.Printf(`
Seeded Sandesh Nagari 7 — Shiv Rudra Group, Nagpur.

  823 plots across 4 sectors (1-302, 303-364, 365-517, 518-823)
  %d one-time maintenance bills (Rs %.2f/sq ft baseline, per-sector) — Rs %.0f collected, Rs %.0f outstanding
  11 ledger entries, 6 progress posts, 4 owner queries (one past SLA), 4 guest leads

  Builder admin : office@shivrudragroup.in
  Site staff    : sitedesk@shivrudragroup.in
  Plot owner    : rohit.deshmukh@example.com   (Plot 147)
  Plot owner    : anjali.kulkarni@example.com  (Plot 412)
  Password      : %s

  Society id    : %s
  Society slug  : sandesh-nagari-7

Ownership, dues, ledger amounts and queries are invented demo data.
Plot numbers, sector ranges, areas and the RERA number are from the brochure.
`, billed, math.Round(baselineRatePerSqft*100)/100, collected, outstanding, password, societyID)
	return nil
}
