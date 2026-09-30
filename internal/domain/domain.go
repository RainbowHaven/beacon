package domain

import (
	"encoding/base64"
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

// MonthConfirmation records that data entry for a safe house and reporting
// month was confirmed complete. Fingerprint is the month's data fingerprint at
// that time.
type MonthConfirmation struct {
	ID              int64
	SafeHouseID     int64
	Month           time.Time // first day of the month, UTC
	Fingerprint     string
	ConfirmedBy     *int64
	ConfirmedByName string // display name, or email when there is none
	ConfirmedAt     time.Time
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
