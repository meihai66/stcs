package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

const cookieName = "stcs_auth"

// password 是进入测试页的访问密码,环境变量 STCS_PASSWORD 配置,默认 admin888。
func password() string {
	if v := os.Getenv("STCS_PASSWORD"); v != "" {
		return v
	}
	return "admin888"
}

// expectedToken 由密码派生,作为 cookie 值;无状态校验,重启后仍有效。
func expectedToken() string {
	sum := sha256.Sum256([]byte(password() + "::stcs-gate-v1"))
	return hex.EncodeToString(sum[:])
}

func tokenValid(t string) bool {
	exp := expectedToken()
	return subtle.ConstantTimeCompare([]byte(t), []byte(exp)) == 1
}

// isAuthed 判断请求是否已通过密码门。
func isAuthed(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	return tokenValid(c.Value)
}

// handleLogin 校验密码,通过则下发 cookie。
func handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(password())) != 1 {
		writeErr(w, http.StatusUnauthorized, "密码错误")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    expectedToken(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		MaxAge:   30 * 24 * 3600,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleLogout 清除 cookie。
func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAuthStatus 报告是否已认证(供前端决定显示密码门还是主界面)。
func handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"authed": isAuthed(r), "required": true})
}

// requireAuth 包裹需要密码门保护的处理器。
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAuthed(r) {
			writeErr(w, http.StatusUnauthorized, "未登录或登录已过期,请输入访问密码。")
			return
		}
		next(w, r)
	}
}
