package model

import "time"

type Permission uint64

const (
	PermRead Permission = 1 << iota
	PermWrite
	PermDelete
	PermShare
)

type User struct {
	ID          uint       `gorm:"primaryKey"`
	Username    string     `gorm:"uniqueIndex;size:64;not null"`
	Password    string     `gorm:"size:255;not null"`
	Permissions Permission `gorm:"default:1"`
	CreatedAt   time.Time
}

func (u *User) HasPerm(p Permission) bool {
	return u.Permissions&p > 0
}

func (u *User) Grant(p Permission) {
	u.Permissions |= p
}

func (u *User) Revoke(p Permission) {
	u.Permissions &= ^p
}
