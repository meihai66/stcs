package httpapi

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/meihai66/stcs/internal/auth"
	"github.com/meihai66/stcs/internal/config"
	"github.com/meihai66/stcs/internal/generator"
	"github.com/meihai66/stcs/internal/reqlog"
	"github.com/meihai66/stcs/internal/store"
)

// ----------------------------- 用户管理(管理员) -----------------------------

func listUsers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"users": store.ListUsers()})
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		Role       string `json:"role"`
		ImageLimit int    `json:"image_limit"`
	}
	_ = decodeJSON(r, &body)
	username := strings.TrimSpace(body.Username)
	if username == "" || strings.TrimSpace(body.Password) == "" {
		writeErr(w, http.StatusBadRequest, "用户名和密码不能为空")
		return
	}
	role := "user"
	if body.Role == "admin" {
		role = "admin"
	}
	limit := body.ImageLimit
	if limit <= 0 {
		limit, _ = strconv.Atoi(store.GetSetting("default_image_limit", "200"))
		if limit <= 0 {
			limit = 200
		}
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "密码处理失败")
		return
	}
	id, err := store.CreateUser(username, hash, role, limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u, _ := store.GetUser(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": u})
}

func updateUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	target, ok := store.GetUser(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "用户不存在")
		return
	}
	var body struct {
		Password   *string `json:"password"`
		Role       *string `json:"role"`
		ImageLimit *int    `json:"image_limit"`
	}
	_ = decodeJSON(r, &body)
	if body.Password != nil && strings.TrimSpace(*body.Password) != "" {
		if hash, err := auth.HashPassword(*body.Password); err == nil {
			store.UpdateUserPassword(id, hash)
			store.DeleteUserSessions(id) // 改密后踢掉该用户所有旧会话
		}
	}
	if body.ImageLimit != nil && *body.ImageLimit >= 0 {
		store.UpdateUserImageLimit(id, *body.ImageLimit)
	}
	if body.Role != nil {
		newRole := "user"
		if *body.Role == "admin" {
			newRole = "admin"
		}
		// 不允许把最后一个管理员降级为普通用户。
		if target.Role == "admin" && newRole != "admin" && store.CountAdmins() <= 1 {
			writeErr(w, http.StatusBadRequest, "不能降级最后一个管理员")
			return
		}
		store.UpdateUserRole(id, newRole)
	}
	u, _ := store.GetUser(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": u})
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	target, ok := store.GetUser(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "用户不存在")
		return
	}
	if id == currentUser(r).ID {
		writeErr(w, http.StatusBadRequest, "不能删除自己")
		return
	}
	if target.Role == "admin" && store.CountAdmins() <= 1 {
		writeErr(w, http.StatusBadRequest, "不能删除最后一个管理员")
		return
	}
	store.DeleteUser(id)
	_ = os.RemoveAll(generator.UserDir(id))
	reqlog.Clear(id, false)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// changePassword 任意登录用户修改自己的密码。
func changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	_ = decodeJSON(r, &body)
	if strings.TrimSpace(body.NewPassword) == "" {
		writeErr(w, http.StatusBadRequest, "新密码不能为空")
		return
	}
	u := currentUser(r)
	_, hash, ok := store.GetUserByUsername(u.Username)
	if !ok || !auth.CheckPassword(hash, body.OldPassword) {
		writeErr(w, http.StatusBadRequest, "原密码错误")
		return
	}
	nh, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "密码处理失败")
		return
	}
	store.UpdateUserPassword(u.ID, nh)
	// 踢掉所有旧会话,再为当前用户重新下发一枚,改密后本人不掉线、旧 token 失效。
	store.DeleteUserSessions(u.ID)
	tok, exp := auth.CreateSession(u.ID)
	setSessionCookie(w, tok, exp)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------- 全局设置(管理员) -----------------------------

func getSettings(w http.ResponseWriter, r *http.Request) {
	logLimit, _ := strconv.Atoi(store.GetSetting("log_limit", "1000"))
	if logLimit <= 0 {
		logLimit = reqlog.Cap()
	}
	defImg, _ := strconv.Atoi(store.GetSetting("default_image_limit", "200"))
	writeJSON(w, http.StatusOK, map[string]any{
		"log_limit":           logLimit,
		"default_image_limit": defImg,
		"concurrency":         config.GlobalConcurrency(),
		"has_server_api_key":  config.GlobalServerAPIKey() != "",
	})
}

func setSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LogLimit          *int    `json:"log_limit"`
		DefaultImageLimit *int    `json:"default_image_limit"`
		Concurrency       *int    `json:"concurrency"`
		ServerAPIKey      *string `json:"server_api_key"`
	}
	_ = decodeJSON(r, &body)
	if body.LogLimit != nil && *body.LogLimit > 0 {
		store.SetSetting("log_limit", strconv.Itoa(*body.LogLimit))
		reqlog.SetCap(*body.LogLimit)
	}
	if body.DefaultImageLimit != nil && *body.DefaultImageLimit >= 0 {
		store.SetSetting("default_image_limit", strconv.Itoa(*body.DefaultImageLimit))
	}
	if body.Concurrency != nil && *body.Concurrency > 0 {
		config.SetGlobalConcurrency(*body.Concurrency)
	}
	if body.ServerAPIKey != nil && *body.ServerAPIKey != "" && !strings.HasPrefix(*body.ServerAPIKey, "****") {
		config.SetGlobalServerAPIKey(*body.ServerAPIKey)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
