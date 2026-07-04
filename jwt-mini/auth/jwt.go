package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type TokenType string
type Claims struct {
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	TokenType TokenType
	jwt.RegisteredClaims
}

var (
	DB        *gorm.DB
	SecretKey = []byte("my-secret-key-123")
	Rdb       *redis.Client
)

func InitRedis() {
	Rdb = redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "123456",
	})

	ctx := context.Background()
	if _, err := Rdb.Ping(ctx).Result(); err != nil {
		panic("连接 Redis 失败: " + err.Error())
	}
	fmt.Println("Redis 连接成功")
}

func GenerateAccessToken(userID uint, username string) (string, error) {
	expirationTime := time.Now().Add(15 * time.Minute)

	claims := &Claims{
		UserID:    userID,
		Username:  username,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.New().String(), // <-- 2. 生成 AccessToken 时加入随机 JTI
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(SecretKey)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

func GenerateRefreshToken(userID uint, username string) (string, error) {
	expirationTime := time.Now().Add(7 * 24 * time.Hour)

	claims := &Claims{
		UserID:    userID,
		Username:  username,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.New().String(), // <-- 3. 生成 RefreshToken 时加入随机 JTI
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(SecretKey)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非法的签名方法: %v", token.Header["alg"])
		}
		return SecretKey, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("无效的 Token")
	}

	return claims, nil
}
