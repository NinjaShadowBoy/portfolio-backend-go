package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

const TokenTypeAccess = "ACCESS"

type Claims struct {
	UserID int64    `json:"userId"`
	Roles  []string `json:"roles"`
	Type   string   `json:"type"`
	jwt.RegisteredClaims
}

type JWT struct {
	secret     []byte
	expiration time.Duration
	issuer     string
}

func NewJWT(secret string, expiration time.Duration, issuer string) *JWT {
	return &JWT{
		secret:     []byte(secret),
		expiration: expiration,
		issuer:     issuer,
	}
}

func (j *JWT) GenerateLoginResponse(user db.User) (dto.LoginResponse, error) {
	token, err := j.GenerateToken(user)
	if err != nil {
		return dto.LoginResponse{}, err
	}

	return dto.LoginResponse{
		Token:     token,
		User:      dto.FromUser(user),
		ExpiresIn: int64(j.expiration / time.Millisecond),
	}, nil
}

func (j *JWT) GenerateToken(user db.User) (string, error) {
	now := time.Now()
	roles := []string{user.Role}
	if user.Role == "ADMIN" {
		roles = []string{"ADMIN", "USER"}
	}

	claims := Claims{
		UserID: user.ID,
		Roles:  roles,
		Type:   TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        randomID(),
			Subject:   user.Email,
			Issuer:    j.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.expiration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}

func (j *JWT) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (j *JWT) ExtractBearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	return strings.TrimSpace(r.Header.Get("X-Auth-Token"))
}

func (j *JWT) HasRole(claims *Claims, role string) bool {
	for _, existing := range claims.Roles {
		if existing == role {
			return true
		}
	}
	return false
}
