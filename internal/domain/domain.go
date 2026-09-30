package domain

import (
	"encoding/base64"
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

// ExpenseCategory is a stable key stored in expenses.category.
type ExpenseCategory string

const (
	CategoryFood               ExpenseCategory = "food"
	CategoryHouseholdSupplies  ExpenseCategory = "household_supplies"
	CategoryUtilities          ExpenseCategory = "utilities"
	CategoryRent               ExpenseCategory = "rent"
	CategoryMaintenanceRepairs ExpenseCategory = "maintenance_repairs"
	CategoryTransport          ExpenseCategory = "transport"
	CategoryCommunication      ExpenseCategory = "communication"
	CategoryOther              ExpenseCategory = "other"
)

type ExpenseCategoryOption struct {
	Key   ExpenseCategory
	Label string
}

var expenseCategories = []ExpenseCategoryOption{
	{CategoryFood, "Food"},
	{CategoryHouseholdSupplies, "Household supplies"},
	{CategoryUtilities, "Utilities"},
	{CategoryRent, "Rent"},
	{CategoryMaintenanceRepairs, "Maintenance and repairs"},
	{CategoryTransport, "Transport"},
	{CategoryCommunication, "Communication"},
	{CategoryOther, "Other"},
}

// ExpenseCategories returns categories in display order.
func ExpenseCategories() []ExpenseCategoryOption {
	return append([]ExpenseCategoryOption(nil), expenseCategories...)
}

func ParseExpenseCategory(s string) (ExpenseCategory, bool) {
	for _, c := range expenseCategories {
		if string(c.Key) == s {
			return c.Key, true
		}
	}
	return "", false
}

func (c ExpenseCategory) Label() string {
	for _, o := range expenseCategories {
		if o.Key == c {
			return o.Label
		}
	}
	return string(c)
}

type ExpenseReviewStatus string

const (
	ExpenseSubmitted       ExpenseReviewStatus = "submitted"
	ExpenseReviewed        ExpenseReviewStatus = "reviewed"
	ExpenseNeedsCorrection ExpenseReviewStatus = "needs_correction"
)

var expenseReviewStatuses = []ExpenseReviewStatus{ExpenseSubmitted, ExpenseReviewed, ExpenseNeedsCorrection}

func ExpenseReviewStatuses() []ExpenseReviewStatus {
	return append([]ExpenseReviewStatus(nil), expenseReviewStatuses...)
}

func ParseExpenseReviewStatus(s string) (ExpenseReviewStatus, bool) {
	for _, st := range expenseReviewStatuses {
		if string(st) == s {
			return st, true
		}
	}
	return "", false
}

func (s ExpenseReviewStatus) Label() string {
	switch s {
	case ExpenseSubmitted:
		return "Submitted"
	case ExpenseReviewed:
		return "Reviewed"
	case ExpenseNeedsCorrection:
		return "Needs correction"
	default:
		return string(s)
	}
}

type Expense struct {
	ID                 int64
	SafeHouseID        int64
	AmountCents        int64
	Currency           string
	Merchant           string
	Category           ExpenseCategory
	Note               string // shown as "Description"
	NoReceiptReason    string
	SpentOn            time.Time
	ReceiptContentType *string
	ReceiptBytes       *int
	ReviewStatus       ExpenseReviewStatus
	ReviewedBy         *int64
	ReviewedAt         *time.Time
	ReviewNote         string
	CreatedBy          *int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (e Expense) HasReceipt() bool { return e.ReceiptBytes != nil && *e.ReceiptBytes > 0 }

// Deletable reports whether the expense has not been accepted by a reviewer yet.
func (e Expense) Deletable() bool { return e.ReviewStatus != ExpenseReviewed }

type ExpenseTotals struct {
	SafeHouseID   int64
	SafeHouseName string
	ExpenseCount  int
	ReceiptCount  int
	AmountCents   int64
	Currency      string
}

// Passkey is a WebAuthn credential as shown to people (no key material).
type Passkey struct {
	ID         []byte
	Label      string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// Key is the URL-safe form of the credential ID used in routes.
func (p Passkey) Key() string { return base64.RawURLEncoding.EncodeToString(p.ID) }

type AuditEvent struct {
	ID          int64
	ActorUserID *int64
	ActorEmail  string
	ActorName   string
	Action      string
	SubjectType string
	SubjectID   string
	Meta        map[string]any
	CreatedAt   time.Time
}
