package router

import (
	"jwt-mini/model"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	// 公开接口
	r.POST("/register", Register)
	r.POST("/login", Login)          // Login 单独提出来
	r.POST("/refresh", RefreshToken) // Refresh 接口

	// 受保护接口
	auth := r.Group("/api")
	auth.Use(AuthMiddleware())
	{
		auth.GET("/profile", GetProfile)
		auth.DELETE("/files/:id", RequirePerm(model.PermDelete), DeleteFile)
	}

	return r
}
