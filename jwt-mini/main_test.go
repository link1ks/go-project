package main

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ==================== GenerateToken 测试 ====================

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken(1, "alice")
	if err != nil {
		t.Fatalf("期望成功，但返回了错误: %v", err)
	}
	if token == "" {
		t.Fatal("期望返回非空 Token")
	}
}

func TestGenerateToken_DifferentUsers(t *testing.T) {
	token1, _ := GenerateToken(1, "alice")
	token2, _ := GenerateToken(2, "bob")

	if token1 == token2 {
		t.Fatal("不同用户生成的 Token 应该不同")
	}
}

// ==================== ParseToken 测试 ====================

func TestParseToken_Valid(t *testing.T) {
	// 1. 签发一个有效 Token
	token, _ := GenerateToken(1, "alice")

	// 2. 解析
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("期望成功解析，但返回了错误: %v", err)
	}

	// 3. 验证内容
	if claims.UserID != 1 {
		t.Errorf("期望 UserID=1，实际=%d", claims.UserID)
	}
	if claims.Username != "alice" {
		t.Errorf("期望 Username=alice，实际=%s", claims.Username)
	}
}

func TestParseToken_Expired(t *testing.T) {
	// 签发一个立即过期的 Token
	claims := &Claims{
		UserID:   1,
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)), // 过去的时间
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString(SecretKey)

	// 解析
	_, err := ParseToken(tokenString)
	if err == nil {
		t.Fatal("过期 Token 应该返回错误，但成功了")
	}
	t.Logf("过期 Token 正确返回错误: %v", err)
}

func TestParseToken_InvalidSignature(t *testing.T) {
	// 用一个不同的密钥伪造 Token
	fakeClaims := &Claims{
		UserID:   1,
		Username: "hacker",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	fakeToken := jwt.NewWithClaims(jwt.SigningMethodHS256, fakeClaims)
	fakeKey := []byte("wrong-key")
	fakeTokenString, _ := fakeToken.SignedString(fakeKey)

	// 用正确的 SecretKey 解析
	_, err := ParseToken(fakeTokenString)
	if err == nil {
		t.Fatal("伪造签名 Token 应该返回错误，但成功了")
	}
	t.Logf("伪造签名正确返回错误: %v", err)
}

func TestParseToken_EmptyToken(t *testing.T) {
	_, err := ParseToken("")
	if err == nil {
		t.Fatal("空 Token 应该返回错误")
	}
}

func TestParseToken_MalformedToken(t *testing.T) {
	_, err := ParseToken("this-is-not-a-jwt")
	if err == nil {
		t.Fatal("格式错误 Token 应该返回错误")
	}
}

// ==================== 表驱动测试（最规范的写法）====================

func TestParseToken_TableDriven(t *testing.T) {
	// 准备一个有效 Token
	validToken, _ := GenerateToken(1, "alice")

	// 准备过期 Token
	expiredClaims := &Claims{
		UserID:   1,
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	}
	expiredJWT, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims).SignedString(SecretKey)

	tests := []struct {
		name      string
		token     string
		wantError bool
	}{
		{"有效Token", validToken, false},
		{"过期Token", expiredJWT, true},
		{"空Token", "", true},
		{"错误格式", "abc.def.ghi", true},
		{"Bearer前缀混入", "Bearer " + validToken, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseToken(tt.token)
			if (err != nil) != tt.wantError {
				t.Errorf("ParseToken() error = %v, wantError = %v", err, tt.wantError)
			}
		})
	}
}
