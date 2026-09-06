package wauser

import (
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/magiconair/beacon/internal/domain"
)

// User adapts a domain user + credentials to webauthn.User.
type User struct {
	domain.User
	Credentials []webauthn.Credential
}

func (u User) WebAuthnID() []byte {
	return u.ID[:]
}

func (u User) WebAuthnName() string {
	return u.Email
}

func (u User) WebAuthnDisplayName() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Email
}

func (u User) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

func FromDomain(u domain.User, creds []webauthn.Credential) User {
	return User{User: u, Credentials: creds}
}

func IDFromHandle(handle []byte) (uuid.UUID, error) {
	return uuid.FromBytes(handle)
}
