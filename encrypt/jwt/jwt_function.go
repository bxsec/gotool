package jwt

import (
	"github.com/golang-jwt/jwt/v4"
	"time"
)

type Claims struct {
	UserName string `json:"user_name"`
	jwt.StandardClaims
}

func GenerateToken(username string, issuer string, secret []byte, expireDuration time.Duration) (string, error) {
	nowTime := time.Now()
	expireTime := nowTime.Add(expireDuration)

	claims := Claims{
		username,
		jwt.StandardClaims{
			ExpiresAt: expireTime.Unix(),
			Issuer:    issuer,
		},
	}

	tokenClaims := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := tokenClaims.SignedString(secret)

	return token, err
}

func ParseToken(token string, jwtSecret []byte) (*Claims, error) {
	tokenClaims, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if tokenClaims != nil {
		if claims, ok := tokenClaims.Claims.(*Claims); ok && tokenClaims.Valid {
			return claims, nil
		}
	}
	return nil, err
}
