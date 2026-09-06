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
