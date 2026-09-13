package models

import "time"

// AccessToken is one issued token.
//
// Only the SHA-256 hash of the token is stored, never the token itself: a
// database that leaks then exposes no usable credentials, the same reason
// passwords are hashed. The plaintext is returned to the caller once, at sign-in,
// and is unrecoverable afterwards.
//
// Rows are what make a token revocable — deleting one signs that session out
// immediately, which a self-contained token (a JWT) cannot do without extra
// machinery.
type AccessToken struct {
	BaseModel
	UserID string `json:"user_id" gorm:"column:user_id"`
	// TokenHash is the hex-encoded SHA-256 digest. Unique, so a lookup is an
	// index hit rather than a scan.
	TokenHash string     `json:"-" gorm:"column:token_hash"`
	ExpiresAt *time.Time `json:"expires_at" gorm:"column:expires_at"`
}

func (AccessToken) TableName() string {
	return "access_tokens"
}
