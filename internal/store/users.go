package store

import (
	"database/sql"
	"errors"
	"time"
)

// User 是一个账户(不含密码哈希)。
type User struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	Role       string `json:"role"` // admin | user
	ImageLimit int    `json:"image_limit"`
	CreatedAt  int64  `json:"created_at"`
}

// ErrUserExists 在用户名已存在时返回。
var ErrUserExists = errors.New("用户名已存在")

// CreateUser 新建用户,返回 id。用户名唯一。
func CreateUser(username, passwordHash, role string, imageLimit int) (int64, error) {
	mu.Lock()
	defer mu.Unlock()
	var exists int
	_ = db.QueryRow(`SELECT 1 FROM users WHERE username=?`, username).Scan(&exists)
	if exists == 1 {
		return 0, ErrUserExists
	}
	res, err := db.Exec(`INSERT INTO users (username, password_hash, role, image_limit, created_at) VALUES (?,?,?,?,?)`,
		username, passwordHash, role, imageLimit, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetUserByUsername 返回用户与其密码哈希。
func GetUserByUsername(username string) (User, string, bool) {
	mu.Lock()
	defer mu.Unlock()
	var u User
	var hash string
	err := db.QueryRow(`SELECT id, username, role, image_limit, created_at, password_hash FROM users WHERE username=?`, username).
		Scan(&u.ID, &u.Username, &u.Role, &u.ImageLimit, &u.CreatedAt, &hash)
	if err != nil {
		return User{}, "", false
	}
	return u, hash, true
}

// GetUser 按 id 返回用户。
func GetUser(id int64) (User, bool) {
	mu.Lock()
	defer mu.Unlock()
	var u User
	err := db.QueryRow(`SELECT id, username, role, image_limit, created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Username, &u.Role, &u.ImageLimit, &u.CreatedAt)
	if err != nil {
		return User{}, false
	}
	return u, true
}

// ListUsers 返回全部用户(按 id 升序)。
func ListUsers() []User {
	mu.Lock()
	defer mu.Unlock()
	rows, err := db.Query(`SELECT id, username, role, image_limit, created_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if rows.Scan(&u.ID, &u.Username, &u.Role, &u.ImageLimit, &u.CreatedAt) == nil {
			out = append(out, u)
		}
	}
	return out
}

// CountUsers 返回用户总数(用于判断是否需要引导首个管理员)。
func CountUsers() int {
	mu.Lock()
	defer mu.Unlock()
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n
}

// UpdateUserPassword 改密。
func UpdateUserPassword(id int64, passwordHash string) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, passwordHash, id)
}

// UpdateUserImageLimit 改图片上限。
func UpdateUserImageLimit(id int64, limit int) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`UPDATE users SET image_limit=? WHERE id=?`, limit, id)
}

// UpdateUserRole 改角色。
func UpdateUserRole(id int64, role string) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`UPDATE users SET role=? WHERE id=?`, role, id)
}

// FirstAdminID 返回最早创建的管理员 id(对外 API 用其配置);无则返回 0。
func FirstAdminID() int64 {
	mu.Lock()
	defer mu.Unlock()
	var id int64
	_ = db.QueryRow(`SELECT id FROM users WHERE role='admin' ORDER BY id ASC LIMIT 1`).Scan(&id)
	return id
}

// CountAdmins 返回管理员数量(用于禁止删除/降级最后一个管理员)。
func CountAdmins() int {
	mu.Lock()
	defer mu.Unlock()
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&n)
	return n
}

// DeleteUser 删除用户及其历史/收藏/会话/配置,返回其历史关联的文件名供清盘。
func DeleteUser(id int64) []string {
	mu.Lock()
	defer mu.Unlock()
	files := collectFiles(`SELECT files FROM history WHERE user_id=?`, id)
	_, _ = db.Exec(`DELETE FROM history WHERE user_id=?`, id)
	_, _ = db.Exec(`DELETE FROM favorites WHERE user_id=?`, id)
	_, _ = db.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
	_, _ = db.Exec(`DELETE FROM users WHERE id=?`, id)
	return files
}

// ----------------------------- 会话 -----------------------------

// CreateSession 写入一个会话 token。
func CreateSession(token string, userID, expiresAt int64) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?,?,?,?)`,
		token, userID, time.Now().Unix(), expiresAt)
}

// SessionUser 校验 token,返回对应用户(已过期或不存在则 ok=false)。
func SessionUser(token string) (User, bool) {
	mu.Lock()
	defer mu.Unlock()
	var uid, exp int64
	if db.QueryRow(`SELECT user_id, expires_at FROM sessions WHERE token=?`, token).Scan(&uid, &exp) != nil {
		return User{}, false
	}
	if time.Now().Unix() > exp {
		_, _ = db.Exec(`DELETE FROM sessions WHERE token=?`, token)
		return User{}, false
	}
	var u User
	if db.QueryRow(`SELECT id, username, role, image_limit, created_at FROM users WHERE id=?`, uid).
		Scan(&u.ID, &u.Username, &u.Role, &u.ImageLimit, &u.CreatedAt) != nil {
		return User{}, false
	}
	return u, true
}

// DeleteSession 删除单个会话(登出)。
func DeleteSession(token string) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`DELETE FROM sessions WHERE token=?`, token)
}

// DeleteUserSessions 删除某用户的全部会话(改密/封禁后踢下线)。
func DeleteUserSessions(userID int64) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`DELETE FROM sessions WHERE user_id=?`, userID)
}

// PurgeExpiredSessions 清理过期会话。
func PurgeExpiredSessions() {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
}

// ----------------------------- 设置(全局) -----------------------------

// GetSetting 读全局设置,缺省返回 def。
func GetSetting(key, def string) string {
	mu.Lock()
	defer mu.Unlock()
	var v sql.NullString
	if db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v) != nil || !v.Valid {
		return def
	}
	return v.String
}

// SetSetting 写全局设置。
func SetSetting(key, value string) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`INSERT INTO settings (key, value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
}

// ----------------------------- 每用户中转站配置 -----------------------------

// GetUserConfig 返回某用户的配置 JSON(空则返回空串)。
func GetUserConfig(userID int64) string {
	mu.Lock()
	defer mu.Unlock()
	var v sql.NullString
	if db.QueryRow(`SELECT config FROM users WHERE id=?`, userID).Scan(&v) != nil || !v.Valid {
		return ""
	}
	return v.String
}

// SetUserConfig 保存某用户的配置 JSON。
func SetUserConfig(userID int64, cfgJSON string) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`UPDATE users SET config=? WHERE id=?`, cfgJSON, userID)
}

// ReassignLegacyData 把历史遗留(user_id=0)的历史/收藏归到指定用户。
func ReassignLegacyData(userID int64) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`UPDATE history SET user_id=? WHERE user_id=0`, userID)
	_, _ = db.Exec(`UPDATE favorites SET user_id=? WHERE user_id=0`, userID)
}
