package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type UserStatus string

const (
	UserPending UserStatus = "pending"
	UserActive  UserStatus = "active"
	UserLocked  UserStatus = "locked"
)

type Role string

const (
	RoleRHCAdmin         Role = "rhc_admin"
	RoleRHLAdmin         Role = "rhl_admin"
	RoleSafeHouseManager Role = "safe_house_manager"
)

type User struct {
	ID          int64
	Email       string
	DisplayName string
	Status      UserStatus
	Role        Role
	RHLID       *int64
	SafeHouseID *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (u User) IsActive() bool { return u.Status == UserActive }

type RHL struct {
	ID     int64
	Name   string
	Code   string // empty when not set
	Active bool
}

// Label is the RHL name followed by its code in parentheses when set.
func (r RHL) Label() string {
	if r.Code == "" {
		return r.Name
	}
	return r.Name + " (" + r.Code + ")"
}

// NormalizeRHLCode trims and upper-cases an RHL code. An empty result means
// no code; otherwise it must be 2–12 characters of A–Z, 0–9 or hyphen.
func NormalizeRHLCode(s string) (string, bool) {
	code := strings.ToUpper(strings.TrimSpace(s))
	if code == "" {
		return "", true
	}
	if len(code) < 2 || len(code) > 12 {
		return "", false
	}
	for _, c := range code {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
			return "", false
		}
	}
	return code, true
}

type SafeHouse struct {
	ID                     int64
	RHLID                  int64
	Name                   string
	DefaultCurrency        string
	ApprovedSleepingPlaces *int
	Active                 bool
}

type Invite struct {
	ID        int64
	UserID    int64
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// Occupant operational fields (nickname + stay dates + reporting demographics).
// No legal identity is stored.
type Occupant struct {
	ID              int64
	SafeHouseID     int64
	Nickname        string
	ArrivedAt       time.Time // date
	DepartedAt      *time.Time
	CountryOfOrigin string // ISO 3166-1 alpha-2 or NR/UNK/XXA
	Gender          string // M, F, X, NR
	BirthYear       *int
	CreatedBy       *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (o Occupant) Current(asOf time.Time) bool {
	if o.DepartedAt == nil {
		return true
	}
	d := o.DepartedAt.UTC().Truncate(24 * time.Hour)
	day := asOf.UTC().Truncate(24 * time.Hour)
	return d.After(day)
}

type HeadcountRow struct {
	SafeHouseID   int64
	SafeHouseName string
	Current       int
}

type Expense struct {
	ID                 int64
	SafeHouseID        int64
	AmountCents        int64
	Currency           string
	Note               string
	SpentOn            time.Time
	ReceiptContentType *string
	ReceiptBytes       *int
	CreatedBy          *int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (e Expense) HasReceipt() bool { return e.ReceiptBytes != nil && *e.ReceiptBytes > 0 }

type ExpenseTotals struct {
	SafeHouseID   int64
	SafeHouseName string
	ExpenseCount  int
	ReceiptCount  int
	AmountCents   int64
	Currency      string
}

type SafeguardingStatus string

const (
	SafeguardingOpen     SafeguardingStatus = "open"
	SafeguardingResolved SafeguardingStatus = "resolved"
	SafeguardingClosed   SafeguardingStatus = "closed"
)

// SafeguardingStatuses lists the statuses in display order.
var SafeguardingStatuses = []SafeguardingStatus{SafeguardingOpen, SafeguardingResolved, SafeguardingClosed}

func (s SafeguardingStatus) Valid() bool {
	switch s {
	case SafeguardingOpen, SafeguardingResolved, SafeguardingClosed:
		return true
	}
	return false
}

// Label is the plain-English status shown in the UI.
func (s SafeguardingStatus) Label() string {
	switch s {
	case SafeguardingOpen:
		return "Open"
	case SafeguardingResolved:
		return "Resolved"
	case SafeguardingClosed:
		return "Closed"
	}
	return string(s)
}

// DatePrecision mirrors the Document 37 date options.
type DatePrecision string

const (
	DateExact       DatePrecision = "exact"
	DateApproximate DatePrecision = "approximate"
	DateUnknown     DatePrecision = "unknown"
)

func (p DatePrecision) Valid() bool {
	switch p {
	case DateExact, DateApproximate, DateUnknown:
		return true
	}
	return false
}

// SafeguardingConcern is tracking data for a Document 37 report. The incident
// narrative and the completed form are never stored in Beacon.
type SafeguardingConcern struct {
	ID                int64
	SafeHouseID       int64
	IncidentID        string
	OccurredOn        *time.Time // nil when OccurredPrecision is unknown
	OccurredPrecision DatePrecision
	ReportedOn        time.Time
	Status            SafeguardingStatus
	ClosedOn          *time.Time // set when Status is not open
	CreatedBy         *int64
	UpdatedBy         *int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// MaxIncidentIDLen is the longest incident identifier Beacon accepts.
const MaxIncidentIDLen = 40

// NormalizeIncidentID trims s and reports whether it is a usable identifier:
// non-empty, at most MaxIncidentIDLen characters and on a single line.
func NormalizeIncidentID(s string) (string, bool) {
	id := strings.TrimSpace(s)
	if id == "" || utf8.RuneCountInString(id) > MaxIncidentIDLen || strings.ContainsAny(id, "\r\n") {
		return "", false
	}
	return id, true
}

// IncidentID formats a Document 37 incident identifier.
func IncidentID(rhlCode string, year, seq int) string {
	return fmt.Sprintf("Incident-%s-%d-%d", rhlCode, year, seq)
}

type AuditEvent struct {
	ID          int64
	ActorUserID *int64
	ActorEmail  string
	Action      string
	SubjectType string
	SubjectID   string
	Meta        map[string]any
	CreatedAt   time.Time
}
