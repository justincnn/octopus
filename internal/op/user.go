package op

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
)

var userCache model.User

const passwordAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomPassword 生成 n 位随机密码(字母+数字), 避免默认 admin/admin 弱口令。
func randomPassword(n int) (string, error) {
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = passwordAlphabet[idx.Int64()]
	}
	return string(b), nil
}

func UserInit() error {
	if err := db.GetDB().First(&userCache).Error; err == nil {
		return nil
	}
	pwd, err := randomPassword(16)
	if err != nil {
		return fmt.Errorf("failed to generate initial password: %w", err)
	}
	userCache.Username = "admin"
	userCache.Password = pwd
	if err := userCache.HashPassword(); err != nil {
		return err
	}
	if err := db.GetDB().Create(&userCache).Error; err != nil {
		return err
	}
	log.Infof("first run: created admin user. Initial password (shown once): %s", pwd)
	return nil
}

func UserChangePassword(oldPassword, newPassword string) error {
	if err := userCache.ComparePassword(oldPassword); err != nil {
		return fmt.Errorf("incorrect old password: %w", err)
	}

	userCache.Password = newPassword
	if err := userCache.HashPassword(); err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	if err := db.GetDB().Model(&userCache).Update("password", userCache.Password).Error; err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

func UserChangeUsername(newUsername string) error {
	if userCache.Username == newUsername {
		return fmt.Errorf("new username is the same as the old username")
	}
	userCache.Username = newUsername
	if err := db.GetDB().Model(&userCache).Update("username", userCache.Username).Error; err != nil {
		return fmt.Errorf("failed to update username: %w", err)
	}
	return nil
}

func UserVerify(username, password string) error {
	if username != userCache.Username {
		return fmt.Errorf("incorrect username")
	}
	if err := userCache.ComparePassword(password); err != nil {
		return fmt.Errorf("incorrect password")
	}
	return nil
}

func UserGet() model.User {
	return userCache
}
