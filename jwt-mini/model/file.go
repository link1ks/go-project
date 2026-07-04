package model

import "time"

type File struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	UserID    uint   `gorm:"index;not null"`
	Filename  string `gorm:"size:255;not null"`
	Size      int64  `gorm:"not null"`
	ObjectKey string `gorm:"size:500;not null;uniqueIndex"`
	CreatedAt time.Time
}
