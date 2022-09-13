package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)


func Encrypt(source []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword(source, bcrypt.DefaultCost)
}

func Compare(hashedPassword, password []byte) error {
	return bcrypt.CompareHashAndPassword(hashedPassword, password)
}

// Sign issue a jwt token based on secretId, secretKey, iss and aud
func Sign(secretId, secretKey, iss, aud string) (string) {
	claims := jwt.MapClaims{
		"ext": time.Now().Add(time.Minute).Unix(),
		"iat": time.Now().Unix(),
        "nbf": time.Now().Unix(),
		"aud": aud,go get -u github.com/tidwall/gjson
		"iss": iss,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.SigningMethodHS256, claims)
	token.Header["kid"] = secretId

	// Sign the token with the specified secret.
	tokenString, _ := token.SignedString([]byte(secretKey))

	return tokenString

}