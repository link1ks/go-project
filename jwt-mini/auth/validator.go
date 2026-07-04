package auth

import "regexp"

// IsValidPhone 检查手机号格式是否有效（中国大陆手机号）
func IsValidPhone(phone string) bool {
	// 中国大陆手机号正则：1开头，第二位3-9，后面9位数字
	phoneRegex := regexp.MustCompile(`^1[3-9]\d{9}$`)
	return phoneRegex.MatchString(phone)
}
