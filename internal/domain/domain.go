// Package domain holds the types shared across modules and the rules about
// who may do what. Keeping roles here means authorisation is one file to audit.
package domain

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleSuperAdmin   Role = "super_admin"    // us: manages builders
	RoleBuilderAdmin Role = "builder_admin"  // the developer/promoter
	RoleBuilderStaff Role = "builder_staff"  // site or sales team
	RoleOwner        Role = "owner"          // plot owner
)

// IsStaff reports whether the role belongs to the builder's side of the app.
func (r Role) IsStaff() bool {
	return r == RoleSuperAdmin || r == RoleBuilderAdmin || r == RoleBuilderStaff
}

// CanManageSociety covers create/edit of plots, fund entries and updates.
func (r Role) CanManageSociety() bool {
	return r == RoleSuperAdmin || r == RoleBuilderAdmin || r == RoleBuilderStaff
}

// CanManageBuilder covers inviting staff and editing builder settings.
func (r Role) CanManageBuilder() bool {
	return r == RoleSuperAdmin || r == RoleBuilderAdmin
}

func (r Role) Valid() bool {
	switch r {
	case RoleSuperAdmin, RoleBuilderAdmin, RoleBuilderStaff, RoleOwner:
		return true
	}
	return false
}

type User struct {
	ID              uuid.UUID  `json:"id"`
	BuilderID       *uuid.UUID `json:"builderId,omitempty"`
	Email           string     `json:"email"`
	Phone           string     `json:"phone,omitempty"`
	Name            string     `json:"name"`
	Role            Role       `json:"role"`
	AvatarURL       string     `json:"avatarUrl,omitempty"`
	CurrentAddress  string     `json:"currentAddress,omitempty"`
	City            string     `json:"city,omitempty"`
	DirectoryOptIn  bool       `json:"directoryOptIn"`
	IsActive        bool       `json:"isActive"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// PlotStatus values mirror the CHECK constraint on plots.status.
const (
	PlotAvailable   = "available"
	PlotBooked      = "booked"
	PlotSold        = "sold"
	PlotOnHold      = "on_hold"
	PlotDisputed    = "disputed"
	PlotNotForSale  = "not_for_sale"
)

func ValidPlotStatus(s string) bool {
	switch s {
	case PlotAvailable, PlotBooked, PlotSold, PlotOnHold, PlotDisputed, PlotNotForSale:
		return true
	}
	return false
}

// QueryCategory values mirror the CHECK constraint on queries.category.
// The SLA attached to each one is what the owner sees as a countdown.
var QuerySLA = map[string]time.Duration{
	"documents_legal":  7 * 24 * time.Hour,
	"payments_dues":    3 * 24 * time.Hour,
	"plot_condition":   5 * 24 * time.Hour,
	"infrastructure":   10 * 24 * time.Hour,
	"construction_noc": 10 * 24 * time.Hour,
	"resale_transfer":  14 * 24 * time.Hour,
	"site_visit":       3 * 24 * time.Hour,
	"other":            7 * 24 * time.Hour,
}

func ValidQueryCategory(c string) bool {
	_, ok := QuerySLA[c]
	return ok
}

func ValidQueryStatus(s string) bool {
	switch s {
	case "open", "in_progress", "waiting_on_owner", "resolved", "closed":
		return true
	}
	return false
}
