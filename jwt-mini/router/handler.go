package router

import (
	"errors"
	"jwt-mini/auth"
	"jwt-mini/model"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 格式错误"})
			c.Abort()
			return
		}
		claims, err := auth.ParseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效或已过期"})
			c.Abort()
			return
		}
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

func RequirePerm(p model.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint("userID")

		var user model.User
		if err := auth.DB.Where("id = ?", userID).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			c.Abort()
			return
		}

		if !user.HasPerm(p) {
			c.JSON(http.StatusForbidden, gin.H{"error": "权限不足"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func Register(c *gin.Context) {
	type RegisterReq struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	var req RegisterReq

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名和密码不能为空"})
		return
	}

	var existing model.User
	err := auth.DB.Take(&existing, "username = ?", req.Username).Error
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	user := model.User{
		Username: req.Username,
		Password: string(hashed),
	}
	if err := auth.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "注册成功",
		"user_id":  user.ID,
		"username": user.Username,
	})
}

func Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	var user model.User
	err := auth.DB.Where("username = ?", req.Username).First(&user).Error
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
		return
	}

	// 签发 Token
	tokenAccess, err := auth.GenerateAccessToken(user.ID, req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TokenAccess 签发失败"})
		return
	}

	tokenRefresh, err := auth.GenerateRefreshToken(user.ID, req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TokenRefresh 签发失败"})
		return
	}
	auth.RefreshStore.Store(tokenRefresh, true)

	c.JSON(http.StatusOK, gin.H{
		"token_access":  tokenAccess,
		"token_refresh": tokenRefresh,
		"message":       "登录成功",
	})
}

func GetProfile(c *gin.Context) {
	userID := c.GetUint("userID")
	username := c.GetString("username")

	c.JSON(http.StatusOK, gin.H{
		"user_id":  userID,
		"username": username,
		"message":  "欢迎回来！",
	})
}

func DeleteFile(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "文件已删除"})
}

func RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	claims, err := auth.ParseToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh Token 无效或已过期"})
		return
	}
	if claims.TokenType != "refresh" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 类型错误"})
		return
	}
	if _, ok := auth.RefreshStore.Load(req.RefreshToken); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh Token 无效或已被使用"})
		return
	}

	auth.RefreshStore.Delete(req.RefreshToken)

	newAccess, err := auth.GenerateAccessToken(claims.UserID, claims.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TokenAccess 签发失败"})
		return
	}

	newRefresh, err := auth.GenerateRefreshToken(claims.UserID, claims.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TokenRefresh 签发失败"})
		return
	}
	auth.RefreshStore.Store(newRefresh, true)

	// 6. 返回
	c.JSON(http.StatusOK, gin.H{
		"access_token":  newAccess,
		"refresh_token": newRefresh,
		"message":       "Token 已刷新",
	})
}
