package storage

import (
	"context"
	"log"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	MinioClient *minio.Client
	BucketName  = "cloud"
)

func InitMinio() {
	var err error
	MinioClient, err = minio.New("127.0.0.1:9000", &minio.Options{
		Creds:  credentials.NewStaticV4("link1k", "/k/k+0k0k", ""),
		Secure: false,
	})
	if err != nil {
		panic("MinIO 连接失败: " + err.Error())
	}

	ctx := context.Background()
	exists, err := MinioClient.BucketExists(ctx, BucketName)
	if err != nil {
		panic("检查 Bucket 失败: " + err.Error())
	}
	if !exists {
		log.Printf("MinIO Bucket '%s' 桶不存在", BucketName)
		log.Printf("正在创建 MinIO Bucket '%s' 桶...", BucketName)
		if err = MinioClient.MakeBucket(ctx, BucketName, minio.MakeBucketOptions{}); err != nil {
			panic("创建 Bucket 失败: " + err.Error())
		}
		log.Printf("MinIO Bucket '%s' 创建成功", BucketName)
	}
}

func PresignPut(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error) {
	return MinioClient.PresignedPutObject(ctx, BucketName, objectKey, expiry)
}

func PresignGet(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error) {
	return MinioClient.PresignedGetObject(ctx, BucketName, objectKey, expiry, nil)
}
