package wauser

import (
	"encoding/binary"
	"fmt"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/go-webauthn/webauthn/webauthn"
)

// User adapts a domain user + credentials to webauthn.User.
type User struct {
	domain.User
	Credentials []webauthn.Credential
}

func (u User) WebAuthnID() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(u.ID))
	return b
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

func IDFromHandle(handle []byte) (int64, error) {
	if len(handle) != 8 {
		return 0, fmt.Errorf("webauthn user handle: want 8 bytes, got %d", len(handle))
	}
	return int64(binary.BigEndian.Uint64(handle)), nil
}
