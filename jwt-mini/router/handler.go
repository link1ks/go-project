package router

import (
	"errors"
	"fmt"
	"jwt-mini/auth"
	"jwt-mini/model"
	"jwt-mini/storage"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
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
		Phone    string `json:"phone"`
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

	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	user := model.User{
		Username:    req.Username,
		Password:    hashed,
		Permissions: model.PermRead | model.PermWrite | model.PermDelete,
	}
	if req.Phone != "" {
		user.Phone = &req.Phone
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
	if !auth.CheckPassword(user.Password, req.Password) {
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

	auth.Rdb.Set(c, tokenRefresh, "valid", 7*24*time.Hour)

	c.JSON(http.StatusOK, gin.H{
		"token_access":  tokenAccess,
		"token_refresh": tokenRefresh,
		"message":       "登录成功",
	})
}

func GetProfile(c *gin.Context) {
	userID := c.GetUint("userID")

	var user model.User
	if err := auth.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	var fileCount int64
	auth.DB.Model(&model.File{}).Where("user_id = ?", userID).Count(&fileCount)

	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":     user.ID,
		"username":    user.Username,
		"phone":       phone,
		"created_at":  user.CreatedAt,
		"permissions": user.Permissions,
		"file_count":  fileCount,
	})
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

	if _, err := auth.Rdb.Get(c, req.RefreshToken).Result(); errors.Is(err, redis.Nil) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh Token 无效或已被使用"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
		return
	}

	auth.Rdb.Del(c, req.RefreshToken)

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

	auth.Rdb.Set(c, newRefresh, "valid", 7*24*time.Hour)

	c.JSON(http.StatusOK, gin.H{
		"access_token":  newAccess,
		"refresh_token": newRefresh,
		"message":       "Token 已刷新",
	})
}

// SendSmsCode 发送验证码
func SendSmsCode(c *gin.Context) {
	var req struct {
		Phone string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if !auth.IsValidPhone(req.Phone) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "手机号格式无效"})
		return
	}

	rateKey := "sms_rate:" + req.Phone
	if err := auth.Rdb.Get(c, rateKey).Err(); err == nil {
		ttl, _ := auth.Rdb.TTL(c, rateKey).Result()
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": fmt.Sprintf("请 %v 秒后再试", ttl.Seconds()),
		})
		return
	}

	code := fmt.Sprintf("%06d", rand.Intn(1000000))

	key := "sms_code:" + req.Phone
	auth.Rdb.Set(c, rateKey, "1", time.Minute)
	auth.Rdb.Set(c, key, code, 5*time.Minute)

	fmt.Printf("【验证码】手机号: %s, 验证码: %s\n", req.Phone, code)

	c.JSON(http.StatusOK, gin.H{"message": "验证码已发送"})
}

func SmsLogin(c *gin.Context) {
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	key := "sms_code:" + req.Phone
	storedCode, err := auth.Rdb.Get(c, key).Result()
	if errors.Is(err, redis.Nil) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "验证码已过期或未发送"})
		return
	}

	if storedCode != req.Code {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "验证码错误"})
		return
	}

	auth.Rdb.Del(c, key)

	var user model.User
	result := auth.DB.Where("phone = ?", req.Phone).First(&user).Error
	if errors.Is(result, gorm.ErrRecordNotFound) {
		user := model.User{
			Username: req.Phone,
			Phone:    &req.Phone,
		}
		auth.DB.Create(&user)
	}

	accessToken, _ := auth.GenerateAccessToken(user.ID, user.Username)
	refreshToken, _ := auth.GenerateRefreshToken(user.ID, user.Username)
	auth.Rdb.Set(c, refreshToken, "valid", 7*24*time.Hour)

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"message":       "登录成功",
	})
}

func SetPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码不能为空"})
		return
	}

	userID := c.GetUint("userID")

	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	if err := auth.DB.Model(&model.User{}).Where("id = ?", userID).Update("password", hashed).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置密码失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码设置成功"})
}

// ListFiles 当前用户的文件列表（元数据在 MySQL，按 user_id 过滤）
func ListFiles(c *gin.Context) {
	userID := c.GetUint("userID")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	var total int64
	if err := auth.DB.Model(&model.File{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	var files []model.File
	offset := (page - 1) * size
	if err := auth.DB.Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).
		Limit(size).
		Find(&files).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	items := make([]gin.H, 0, len(files))
	for _, f := range files {
		items = append(items, gin.H{
			"file_id":      f.ID,
			"filename":     f.Filename,
			"size":         f.Size,
			"created_at":   f.CreatedAt,
			"download_api": fmt.Sprintf("/api/files/presign/download/%d", f.ID),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"page":  page,
		"size":  size,
		"total": total,
		"files": items,
	})
}

// RemoveFile 删除文件
func RemoveFile(c *gin.Context) {
	fileID := c.Param("id")
	userID := c.GetUint("userID")

	var file model.File
	if err := auth.DB.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	storage.MinioClient.RemoveObject(c, storage.BucketName, file.ObjectKey, minio.RemoveObjectOptions{})
	auth.DB.Delete(&file)

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// PresignUpload 申请预签名上传 URL
func PresignUpload(c *gin.Context) {
	userID := c.GetUint("userID")

	var req struct {
		Filename string `json:"filename"`
	}

	if err := c.ShouldBindJSON(&req); err != nil || req.Filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件名输入错误"})
		return
	}

	objectKey := fmt.Sprintf("%d/%s_%s", userID, uuid.New().String(), req.Filename)

	url, err := storage.PresignPut(c, objectKey, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "上传失败"})
		return
	}

	c.PureJSON(http.StatusOK, gin.H{
		"upload_url": url.String(),
		"object_key": objectKey,
		"expires_in": 900,
	})
}

// ConfirmUpload 确认入库
func ConfirmUpload(c *gin.Context) {
	userID := c.GetUint("userID")

	var req struct {
		Filename  string `json:"filename"`
		ObjectKey string `json:"object_key"`
		Size      int64  `json:"size"`
	}

	if err := c.ShouldBindJSON(&req); err != nil || req.Filename == "" || req.ObjectKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if !strings.HasPrefix(req.ObjectKey, fmt.Sprintf("%d/", userID)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "objectKey 输入错误"})
		return
	}

	info, err := storage.MinioClient.StatObject(c, storage.BucketName, req.ObjectKey, minio.StatObjectOptions{})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "对象不存在"})
		return
	}

	req.Size = info.Size

	var existing model.File
	err = auth.DB.Where("object_key = ?", req.ObjectKey).First(&existing).Error
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "文件已入库",
			"file_id": existing.ID,
		})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	file := model.File{
		UserID:    userID,
		Filename:  req.Filename,
		Size:      req.Size,
		ObjectKey: req.ObjectKey,
	}
	if err = auth.DB.Create(&file).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}

	c.PureJSON(http.StatusOK, gin.H{"file_id": file.ID})
}

// PresignDownload 预签名下载
func PresignDownload(c *gin.Context) {
	userID := c.GetUint("userID")
	fileID := c.Param("id")

	var file model.File
	if err := auth.DB.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	url, err := storage.PresignGet(c, file.ObjectKey, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "下载失败"})
		return
	}

	c.PureJSON(http.StatusOK, gin.H{
		"download_url": url.String(),
		"expires_in":   900,
	})
}

func ShareFile(c *gin.Context) {
	userID := c.GetUint("userID")
	fileID := c.Param("id")

	var file model.File
	if err := auth.DB.Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	var req struct {
		ExpiresIn int `json:"expires_in"`
	}
	_ = c.ShouldBindJSON(&req)
	expiresIn := req.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	if expiresIn > 7*24*3600 {
		expiresIn = 7 * 24 * 3600
	}

	token := uuid.New().String()
	key := "file_share:" + token
	if err := auth.Rdb.Set(c, key, file.ID, time.Duration(expiresIn)*time.Second).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建分享失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"share_token": token,
		"share_url":   fmt.Sprintf("http://%s/share/%s", c.Request.Host, token),
		"filename":    file.Filename,
		"expires_in":  expiresIn,
	})
}

func GetSharedFile(c *gin.Context) {
	token := c.Param("token")

	key := "file_share:" + token

	fileID, err := auth.Rdb.Get(c, key).Uint64()
	if errors.Is(err, redis.Nil) {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享链接不存在或已过期"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
		return
	}

	var file model.File
	if err = auth.DB.First(&file, fileID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	url, err := storage.PresignGet(c, file.ObjectKey, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "下载失败"})
		return
	}

	c.PureJSON(http.StatusOK, gin.H{
		"filename":     file.Filename,
		"download_url": url.String(),
		"expires_in":   900,
	})
}
