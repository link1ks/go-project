package router

import (
	"jwt-mini/model"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()
	r.Use(CORSMiddleware())

	// 公开接口
	r.POST("/register", Register)
	r.POST("/login", Login)          // Login 单独提出来
	r.POST("/refresh", RefreshToken) // Refresh 接口
	r.POST("/sms/send", SendSmsCode) // 发送短信验证
	r.POST("/login/sms", SmsLogin)   // 短信登录
	r.GET("/share/:token", GetSharedFile)

	// 受保护接口
	api := r.Group("/api")
	api.Use(AuthMiddleware())
	{
		api.GET("/profile", GetProfile)
		api.POST("/user/password", SetPassword)
		api.GET("/files", ListFiles)
		api.DELETE("/files/remove/:id", RequirePerm(model.PermDelete), RemoveFile)

		api.POST("/files/presign/upload", PresignUpload)
		api.POST("/files/confirm", ConfirmUpload)
		api.GET("/files/presign/download/:id", PresignDownload)
		api.POST("/files/:id/share", RequirePerm(model.PermShare), ShareFile)
	}

	return r
}
