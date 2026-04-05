package auth

import (
	"errors"
	"time"

	"avito/internal/model"

	"github.com/golang-jwt/jwt/v5"
)

const (
	DummyAdminUserID = "11111111-1111-1111-1111-111111111111"
	DummyUserUserID  = "22222222-2222-2222-2222-222222222222"
)

type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func IssueToken(secret string, role model.Role) (string, error) {
	var userID string
	switch role {
	case model.RoleAdmin:
		userID = DummyAdminUserID
	case model.RoleUser:
		userID = DummyUserUserID
	default:
		return "", errors.New("invalid role")
	}

	claims := Claims{
		UserID: userID,
		Role:   string(role),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseToken(secret, tokenString string) (model.UserClaims, error) {
	var claims Claims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return model.UserClaims{}, err
	}
	if !token.Valid {
		return model.UserClaims{}, errors.New("invalid token")
	}
	return model.UserClaims{UserID: claims.UserID, Role: model.Role(claims.Role)}, nil
}
