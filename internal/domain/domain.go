package domain

import (
	"time"

	"github.com/google/uuid"
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
	ID          uuid.UUID
	Email       string
	DisplayName string
	Status      UserStatus
	Role        Role
	RHLID       *uuid.UUID
	SafeHouseID *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (u User) IsActive() bool { return u.Status == UserActive }

type RHL struct {
	ID   uuid.UUID
	Name string
}

type SafeHouse struct {
	ID    uuid.UUID
	RHLID uuid.UUID
	Name  string
}

type Invite struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// Occupant operational fields are plaintext for headcount.
// Legal name / refugee ID live only in IdentityCiphertext (sealed box).
type Occupant struct {
	ID                  uuid.UUID
	SafeHouseID         uuid.UUID
	Nickname            string
	ArrivedAt           time.Time // date
	DepartedAt          *time.Time
	IdentityCiphertext  []byte
	KeyID               string
	CreatedBy           *uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
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
	SafeHouseID   uuid.UUID
	SafeHouseName string
	Current       int
}
