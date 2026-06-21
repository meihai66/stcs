package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/meihai66/stcs/internal/auth"
	"github.com/meihai66/stcs/internal/store"
)

const cookieName = "stcs_session"

// setSessionCookie 下发会话 cookie(登录 / 改密后复用)。
func setSessionCookie(w http.ResponseWriter, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  exp,
		MaxAge:   int(auth.SessionTTL() / time.Second),
	})
}

// sameOrigin 已按需求放宽:允许跨站请求(自托管内网工具,需从其他来源/页面调用接口)。
// 始终放行,不再校验 Origin 与本站是否同源。
func sameOrigin(r *http.Request) bool {
	return true
}

// ---- 登录失败限速(按用户名,5 次失败后冷却 30 秒)----
var (
	loginMu    sync.Mutex
	loginFails = map[string]*failRec{}
)

type failRec struct {
	count int
	until time.Time
}

func loginThrottled(key string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	r := loginFails[key]
	return r != nil && time.Now().Before(r.until)
}

func loginFailed(key string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	r := loginFails[key]
	if r == nil {
		r = &failRec{}
		loginFails[key] = r
	}
	r.count++
	if r.count >= 5 {
		r.until = time.Now().Add(30 * time.Second)
		r.count = 0
	}
}

func loginCleared(key string) {
	loginMu.Lock()
	delete(loginFails, key)
	loginMu.Unlock()
}

type ctxKey int

const userKey ctxKey = 0

// currentUser 取出当前请求的登录用户(经 requireAuth 注入)。
func currentUser(r *http.Request) store.User {
	u, _ := r.Context().Value(userKey).(store.User)
	return u
}

func isAdmin(r *http.Request) bool { return currentUser(r).Role == "admin" }

// requireAuth 校验会话 cookie,注入当前用户。
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			writeErr(w, http.StatusForbidden, "跨站请求被拒绝")
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "未登录或登录已过期,请重新登录。")
			return
		}
		u, ok := store.SessionUser(c.Value)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "未登录或登录已过期,请重新登录。")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}

// requireAdmin 在 requireAuth 基础上要求管理员角色。
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r) {
			writeErr(w, http.StatusForbidden, "需要管理员权限")
			return
		}
		next(w, r)
	})
}

// handleCaptcha 下发一个图形验证码。
func handleCaptcha(w http.ResponseWriter, r *http.Request) {
	id, dataURI := auth.NewCaptcha()
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "image": dataURI})
}

// handleLogin 校验验证码 + 账密,通过则建会话并下发 cookie。
func handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		CaptchaID string `json:"captcha_id"`
		Captcha   string `json:"captcha"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	username := strings.TrimSpace(body.Username)
	if loginThrottled(username) {
		writeErr(w, http.StatusTooManyRequests, "登录失败次数过多,请 30 秒后再试。")
		return
	}
	if !auth.VerifyCaptcha(body.CaptchaID, strings.TrimSpace(body.Captcha)) {
		loginFailed(username)
		writeErr(w, http.StatusBadRequest, "验证码错误或已过期")
		return
	}
	u, hash, ok := store.GetUserByUsername(username)
	if !ok || !auth.CheckPassword(hash, body.Password) {
		loginFailed(username)
		writeErr(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	loginCleared(username)
	tok, exp := auth.CreateSession(u.ID)
	setSessionCookie(w, tok, exp)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": u})
}

// handleLogout 删除会话并清除 cookie。
func handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAuthStatus 报告登录态与当前用户(供前端决定显示登录页还是主界面)。
func handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		if u, ok := store.SessionUser(c.Value); ok {
			writeJSON(w, http.StatusOK, map[string]any{"authed": true, "required": true, "user": u})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"authed": false, "required": true})
}
