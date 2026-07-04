package main

import (
	"fmt"
	"jwt-mini/auth"
	"jwt-mini/router"
	"jwt-mini/storage"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Connect() {
	dsn := "root:/k/k+0k0k@tcp(127.0.0.1:3306)/link_school?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("连接数据库失败")
	}
	auth.DB = db
}

func AutoMigrate(model ...interface{}) {
	err := auth.DB.AutoMigrate(model...)
	if err != nil {
		panic("表迁移失败")
	}
	fmt.Println("表迁移成功")
}

func apply() {
	Connect()
	auth.InitRedis()
	//AutoMigrate(&model.User{}, &model.File{})
	storage.InitMinio()
	r := router.SetupRouter()

	r.Run(":8080")
}
