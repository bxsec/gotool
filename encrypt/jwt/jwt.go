package jwt

import (
	"time"
)

func NewJwter(secret []byte, issuer string) *Jwtor {
	return &Jwtor{
		jwtSecret: secret,
		issuer:    issuer,
	}
}

type Jwtor struct {
	jwtSecret []byte
	issuer    string
}

func (jt *Jwtor) GenerateToken(username string, expireDuration time.Duration) (string, error) {
	return GenerateToken(username, jt.issuer, jt.jwtSecret, expireDuration)
}

func (jt *Jwtor) ParseToken(token string) (*Claims, error) {
	return ParseToken(token, jt.jwtSecret)
}
