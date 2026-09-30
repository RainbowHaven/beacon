package domain

import (
	"strings"
	"time"
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

const (
	OperationalIssueOpen     = "open"
	OperationalIssueResolved = "resolved"
	OperationalIssueClosed   = "closed"
)

type OperationalIssueCategory struct {
	Key   string
	Label string
}

// OperationalIssueCategories keys are stored in the database; labels are display only.
var OperationalIssueCategories = []OperationalIssueCategory{
	{"building_maintenance", "Building or maintenance problem"},
	{"utilities", "Utilities"},
	{"safety_security", "Safety or security issue that is not a safeguarding concern"},
	{"food_supplies", "Food or supplies"},
	{"staffing_agent", "Staffing or Agent change"},
	{"capacity_occupancy", "Capacity or occupancy change"},
	{"service_availability", "Service availability"},
	{"other_change", "Other significant operational change"},
}

func OperationalIssueCategoryLabel(key string) string {
	for _, c := range OperationalIssueCategories {
		if c.Key == key {
			return c.Label
		}
	}
	return key
}

func ValidOperationalIssueCategory(key string) bool {
	for _, c := range OperationalIssueCategories {
		if c.Key == key {
			return true
		}
	}
	return false
}

func ValidOperationalIssueStatus(s string) bool {
	return s == OperationalIssueOpen || s == OperationalIssueResolved || s == OperationalIssueClosed
}

// OperationalIssue is a problem or significant change at a safe house.
// It must not contain residents' names or safeguarding details.
type OperationalIssue struct {
	ID            int64
	SafeHouseID   int64
	SafeHouseName string
	IdentifiedOn  time.Time // date
	Category      string
	Description   string
	Effect        string
	ActionTaken   string
	RHLRequest    string
	Status        string
	ClosedOn      *time.Time // resolution or closure date
	ClosureNotes  string
	CreatedBy     *int64
	UpdatedBy     *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (o OperationalIssue) CategoryLabel() string { return OperationalIssueCategoryLabel(o.Category) }

func (o OperationalIssue) StatusLabel() string { return OperationalIssueStatusLabel(o.Status) }

func OperationalIssueStatusLabel(s string) string {
	switch s {
	case OperationalIssueOpen:
		return "Open"
	case OperationalIssueResolved:
		return "Resolved"
	case OperationalIssueClosed:
		return "Closed"
	default:
		return s
	}
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
