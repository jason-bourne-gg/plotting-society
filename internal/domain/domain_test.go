package domain

import (
	"testing"
	"time"
)

func TestRoleIsStaff(t *testing.T) {
	cases := map[Role]bool{
		RoleSuperAdmin:   true,
		RoleBuilderAdmin: true,
		RoleBuilderStaff: true,
		RoleOwner:        false,
		Role("nonsense"):  false,
	}
	for role, want := range cases {
		if got := role.IsStaff(); got != want {
			t.Errorf("%q.IsStaff() = %v, want %v", role, got, want)
		}
	}
}

func TestRoleCanManageSociety(t *testing.T) {
	cases := map[Role]bool{
		RoleSuperAdmin:   true,
		RoleBuilderAdmin: true,
		RoleBuilderStaff: true,
		RoleOwner:        false,
		Role(""):         false,
	}
	for role, want := range cases {
		if got := role.CanManageSociety(); got != want {
			t.Errorf("%q.CanManageSociety() = %v, want %v", role, got, want)
		}
	}
}

// Site staff run the desk but must not be able to mint another admin account.
func TestRoleCanManageBuilder(t *testing.T) {
	cases := map[Role]bool{
		RoleSuperAdmin:   true,
		RoleBuilderAdmin: true,
		RoleBuilderStaff: false,
		RoleOwner:        false,
		Role("x"):        false,
	}
	for role, want := range cases {
		if got := role.CanManageBuilder(); got != want {
			t.Errorf("%q.CanManageBuilder() = %v, want %v", role, got, want)
		}
	}
}

func TestRoleValid(t *testing.T) {
	for _, role := range []Role{RoleSuperAdmin, RoleBuilderAdmin, RoleBuilderStaff, RoleOwner} {
		if !role.Valid() {
			t.Errorf("%q should be valid", role)
		}
	}
	for _, role := range []Role{"", "admin", "OWNER", "builder"} {
		if Role(role).Valid() {
			t.Errorf("%q should not be valid", role)
		}
	}
}

func TestValidPlotStatus(t *testing.T) {
	for _, s := range []string{PlotAvailable, PlotBooked, PlotSold, PlotOnHold, PlotDisputed, PlotNotForSale} {
		if !ValidPlotStatus(s) {
			t.Errorf("%q should be a valid plot status", s)
		}
	}
	for _, s := range []string{"", "SOLD", "reserved", "sold "} {
		if ValidPlotStatus(s) {
			t.Errorf("%q should not be a valid plot status", s)
		}
	}
}

// Every category in the CHECK constraint needs an SLA, or a query would be
// created with a zero deadline and read as breached the moment it is raised.
func TestQuerySLACoversEveryCategory(t *testing.T) {
	categories := []string{
		"documents_legal", "payments_dues", "plot_condition", "infrastructure",
		"construction_noc", "resale_transfer", "site_visit", "other",
	}
	if len(QuerySLA) != len(categories) {
		t.Fatalf("QuerySLA has %d entries, expected %d", len(QuerySLA), len(categories))
	}
	for _, c := range categories {
		sla, ok := QuerySLA[c]
		if !ok {
			t.Errorf("category %q has no SLA", c)
			continue
		}
		if sla <= 0 {
			t.Errorf("category %q has a non-positive SLA of %v", c, sla)
		}
		if sla > 30*24*time.Hour {
			t.Errorf("category %q has an SLA of %v, which is longer than anyone will wait", c, sla)
		}
	}
}

func TestValidQueryCategory(t *testing.T) {
	if !ValidQueryCategory("documents_legal") {
		t.Error("documents_legal should be valid")
	}
	for _, c := range []string{"", "legal", "DOCUMENTS_LEGAL"} {
		if ValidQueryCategory(c) {
			t.Errorf("%q should not be valid", c)
		}
	}
}

func TestValidQueryStatus(t *testing.T) {
	for _, s := range []string{"open", "in_progress", "waiting_on_owner", "resolved", "closed"} {
		if !ValidQueryStatus(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "done", "OPEN"} {
		if ValidQueryStatus(s) {
			t.Errorf("%q should not be valid", s)
		}
	}
}

func TestSafeExternalURL(t *testing.T) {
	safe := []string{
		"",
		"https://pub-abc.r2.dev/updates/x.jpg",
		"http://localhost:9000/plotting-media/a.pdf",
		"https://example.in/a%20b.pdf",
	}
	for _, u := range safe {
		if !SafeExternalURL(u) {
			t.Errorf("%q should be accepted", u)
		}
	}

	// The whole reason this function exists.
	unsafe := []string{
		"javascript:alert(1)",
		"JavaScript:alert(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
		"//evil.example.com/x.js",
		"/relative/path.jpg",
		"https://",
		"ht tp://bad",
		string(rune(0x7f)) + "://x",
	}
	for _, u := range unsafe {
		if SafeExternalURL(u) {
			t.Errorf("%q should be rejected", u)
		}
	}

	if SafeExternalURL("https://example.com/" + string(make([]byte, 2100))) {
		t.Error("an over-long URL should be rejected")
	}
}
