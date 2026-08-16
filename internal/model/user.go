package model

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID               uint   `gorm:"primaryKey"`
	Username         string `gorm:"unique"`
	Password         string `gorm:"not null"`
	TwoFactorSecret  string `gorm:"column:two_factor_secret;default:''"` // TOTP 密钥, 开启两步验证后非空
}

type UserLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Expire   int    `json:"expire"`
	Code     string `json:"code"` // TOTP 两步验证码, 未开启时留空
}

type UserChangePassword struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type UserChangeUsername struct {
	NewUsername string `json:"new_username"`
}

// TwoFactorSetupResponse 绑定两步验证第一步的返回值。
type TwoFactorSetupResponse struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
	QRCode string `json:"qr_code"` // data:image/png;base64,... 可直接用作 <img src>
}

// TwoFactorCodeRequest 启用/关闭两步验证都需要提供当前 TOTP 验证码。
type TwoFactorCodeRequest struct {
	Code string `json:"code"`
}

// ServerTimeResponse 供登录页在未登录状态下诊断 TOTP 时间偏差。
type ServerTimeResponse struct {
	ServerTime       string `json:"server_time"`
	Timezone         string `json:"timezone"`
	TwoFactorEnabled bool   `json:"two_factor_enabled"`
}

func (u *User) HashPassword() error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	u.Password = string(hashedPassword)
	return nil
}

func (u *User) ComparePassword(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
}
