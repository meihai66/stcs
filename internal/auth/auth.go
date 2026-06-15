// Package auth 负责账户密码(bcrypt)、会话 token、图形验证码与首个管理员引导。
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/meihai66/stcs/internal/store"
)

// sessionTTL 是登录有效期(30 天)。
const sessionTTL = 30 * 24 * time.Hour

// HashPassword 用 bcrypt 哈希明文密码。
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword 校验明文密码与哈希是否匹配。
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// NewToken 生成随机会话 token。
func NewToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateSession 为用户创建会话,返回 token 与过期时间。
func CreateSession(userID int64) (string, time.Time) {
	tok := NewToken()
	exp := time.Now().Add(sessionTTL)
	store.CreateSession(tok, userID, exp.Unix())
	return tok, exp
}

// SessionTTL 暴露给 cookie 设置 MaxAge。
func SessionTTL() time.Duration { return sessionTTL }

// Bootstrap 首次运行时引导首个管理员,并把遗留数据/配置归入它。
// legacyConfig 为旧全局 config.json 内容(可空)。幂等:已有用户则跳过。
func Bootstrap(legacyConfig string) (created bool, username string) {
	if store.CountUsers() > 0 {
		return false, ""
	}
	username = envOr("STCS_ADMIN_USER", "admin")
	pw := envOr("STCS_ADMIN_PASSWORD", envOr("STCS_PASSWORD", "admin888"))
	hash, err := HashPassword(pw)
	if err != nil {
		return false, ""
	}
	id, err := store.CreateUser(username, hash, "admin", 200)
	if err != nil {
		return false, ""
	}
	store.ReassignLegacyData(id)
	if strings.TrimSpace(legacyConfig) != "" {
		store.SetUserConfig(id, legacyConfig)
	}
	return true, username
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
