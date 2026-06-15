// Package httpapi 组装所有 HTTP 路由:Web UI 接口(密码门保护)、对外 OpenAI 兼容 API、静态资源。
package httpapi

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meihai66/stcs/internal/config"
	"github.com/meihai66/stcs/internal/generator"
	"github.com/meihai66/stcs/internal/reqlog"
	"github.com/meihai66/stcs/internal/store"
	"github.com/meihai66/stcs/internal/stress"
	"github.com/meihai66/stcs/internal/tasks"
)

const stressMaxConcurrency = 20000

// New 组装并返回完整的 http.Handler。staticFS 是已构建的前端 dist 文件系统。
func New(staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()

	// ---- 认证(不需登录)----
	mux.HandleFunc("GET /api/captcha", handleCaptcha)
	mux.HandleFunc("POST /api/login", handleLogin)
	mux.HandleFunc("POST /api/logout", handleLogout)
	mux.HandleFunc("GET /api/auth/status", handleAuthStatus)

	// ---- 用户管理 + 全局设置(仅管理员)----
	mux.HandleFunc("GET /api/users", requireAdmin(listUsers))
	mux.HandleFunc("POST /api/users", requireAdmin(createUser))
	mux.HandleFunc("POST /api/users/{id}", requireAdmin(updateUser))
	mux.HandleFunc("DELETE /api/users/{id}", requireAdmin(deleteUser))
	mux.HandleFunc("GET /api/settings", requireAdmin(getSettings))
	mux.HandleFunc("POST /api/settings", requireAdmin(setSettings))
	mux.HandleFunc("POST /api/password", requireAuth(changePassword))

	// ---- Web UI 接口(密码门保护)----
	mux.HandleFunc("GET /api/config", requireAuth(getConfig))
	mux.HandleFunc("POST /api/config", requireAuth(setConfig))
	mux.HandleFunc("GET /api/profiles", requireAuth(listProfiles))
	mux.HandleFunc("POST /api/profiles", requireAuth(saveProfile))
	mux.HandleFunc("POST /api/profiles/activate", requireAuth(activateProfile))
	mux.HandleFunc("DELETE /api/profiles/{name}", requireAuth(deleteProfile))

	mux.HandleFunc("POST /api/generate", requireAuth(generate))
	mux.HandleFunc("GET /api/tasks", requireAuth(listTasks))
	mux.HandleFunc("GET /api/tasks/{id}", requireAuth(getTask))
	mux.HandleFunc("DELETE /api/tasks", requireAuth(clearTasks))
	mux.HandleFunc("POST /api/edit", requireAuth(editImage))

	mux.HandleFunc("GET /api/history", requireAuth(listHistory))
	mux.HandleFunc("DELETE /api/history", requireAuth(clearHistory))
	mux.HandleFunc("DELETE /api/history/{id}", requireAuth(deleteHistory))
	mux.HandleFunc("GET /api/favorites", requireAuth(listFavorites))
	mux.HandleFunc("POST /api/favorites", requireAuth(addFavorite))
	mux.HandleFunc("DELETE /api/favorites/{id}", requireAuth(deleteFavorite))

	mux.HandleFunc("POST /api/reverse-prompt", requireAuth(reversePrompt))
	mux.HandleFunc("GET /api/prompt-market", requireAuth(promptMarket))
	mux.HandleFunc("GET /api/gallery", requireAuth(gallery))

	mux.HandleFunc("POST /api/stress/start", requireAuth(stressStart))
	mux.HandleFunc("GET /api/stress/status", requireAuth(stressStatus))
	mux.HandleFunc("POST /api/stress/stop", requireAuth(stressStop))

	mux.HandleFunc("GET /api/request-logs", requireAuth(listRequestLogs))
	mux.HandleFunc("GET /api/request-logs/{id}", requireAuth(getRequestLog))
	mux.HandleFunc("DELETE /api/request-logs", requireAuth(clearRequestLogs))

	// ---- 对外 OpenAI 兼容 API(用 server_api_key 校验,不走密码门)----
	mux.HandleFunc("GET /v1/models", openaiModels)
	mux.HandleFunc("POST /v1/images/generations", openaiGenerations)

	// ---- 静态资源 ----
	mux.HandleFunc("GET /outputs/{uid}/{name}", requireAuth(serveOutput)) // 生成的图片(需登录,按用户隔离)
	mux.Handle("GET /", spaHandler(staticFS))

	return logMiddleware(mux)
}

// ----------------------------- 辅助 -----------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": msg}})
}

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) >= 4 {
		return "****" + k[len(k)-4:]
	}
	return "****"
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ----------------------------- 配置 -----------------------------

func getConfig(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	cfg := config.LoadForUser(u.ID)
	out := map[string]any{
		"base_url":           cfg.BaseURL,
		"model":              cfg.Model,
		"reverse_model":      cfg.ReverseModel,
		"request_format":     cfg.RequestFormat,
		"default_size":       cfg.DefaultSize,
		"default_quality":    cfg.DefaultQuality,
		"timeout":            cfg.Timeout,
		"concurrency":        cfg.Concurrency,
		"api_key":            maskKey(cfg.APIKey),
		"has_api_key":        cfg.APIKey != "",
		"server_api_key":     mask4admin(cfg.ServerAPIKey, u.Role),
		"has_server_api_key": u.Role == "admin" && cfg.ServerAPIKey != "",
		"running_workers":    tasks.RunningWorkers(),
		"active_profile":     cfg.ActiveProfile,
		"username":           u.Username,
		"role":               u.Role,
		"image_limit":        u.ImageLimit,
	}
	writeJSON(w, http.StatusOK, out)
}

func mask4(s string) string {
	if s == "" {
		return ""
	}
	return "****"
}

// mask4admin 仅向管理员暴露「对外密钥是否已设」,普通用户始终空。
func mask4admin(s, role string) string {
	if role != "admin" {
		return ""
	}
	return mask4(s)
}

func setConfig(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := decodeJSON(r, &payload); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	u := currentUser(r)
	updates := map[string]any{}
	for _, k := range []string{"default_size", "default_quality", "timeout"} {
		if v, ok := payload[k]; ok {
			updates[k] = v
		}
	}
	cfg := config.SaveGlobalForUser(u.ID, updates)
	// 全局项(并发数、对外 API 密钥)仅管理员可改。
	if u.Role == "admin" {
		if v, ok := toIntAny(payload["concurrency"]); ok && v > 0 {
			config.SetGlobalConcurrency(v)
		}
		if v, ok := payload["server_api_key"].(string); ok && v != "" && !strings.HasPrefix(v, "****") {
			config.SetGlobalServerAPIKey(v)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "model": cfg.Model})
}

func toIntAny(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	}
	return 0, false
}

func listProfiles(w http.ResponseWriter, r *http.Request) {
	profs, active := config.ListProfilesForUser(currentUser(r).ID)
	out := make([]map[string]any, 0, len(profs))
	for _, p := range profs {
		out = append(out, map[string]any{
			"name":           p.Name,
			"base_url":       p.BaseURL,
			"api_key":        maskKey(p.APIKey),
			"has_api_key":    p.APIKey != "",
			"model":          p.Model,
			"reverse_model":  p.ReverseModel,
			"request_format": orStr(p.RequestFormat, "images"),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": out, "active": active})
}

func saveProfile(w http.ResponseWriter, r *http.Request) {
	var p config.Profile
	if err := decodeJSON(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	if err := config.SaveProfileForUser(currentUser(r).ID, p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func activateProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = decodeJSON(r, &body)
	uid := currentUser(r).ID
	config.SetActiveForUser(uid, strings.TrimSpace(body.Name))
	cfg := config.LoadForUser(uid)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active": cfg.ActiveProfile, "model": cfg.Model})
}

func deleteProfile(w http.ResponseWriter, r *http.Request) {
	config.DeleteProfileForUser(currentUser(r).ID, r.PathValue("name"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------- 生成 -----------------------------

func generate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt        string   `json:"prompt"`
		Prompts       []string `json:"prompts"`
		Size          string   `json:"size"`
		Quality       string   `json:"quality"`
		N             int      `json:"n"`
		Repeat        int      `json:"repeat"`
		RequestFormat string   `json:"request_format"`
		Model         string   `json:"model"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	uid := currentUser(r).ID
	cfg := config.LoadForUser(uid)
	raw := body.Prompts
	if raw == nil {
		raw = []string{body.Prompt}
	}
	var prompts []string
	for _, p := range raw {
		if s := strings.TrimSpace(p); s != "" {
			prompts = append(prompts, s)
		}
	}
	if len(prompts) == 0 {
		writeErr(w, http.StatusBadRequest, "请至少输入一条提示词")
		return
	}
	if len(prompts) > 50 {
		writeErr(w, http.StatusBadRequest, "一次最多 50 条提示词")
		return
	}
	repeat := clamp(body.Repeat, 1, 20)
	n := clamp(body.N, 1, 4)
	fmtMode := orStr(body.RequestFormat, cfg.RequestFormat)
	size := orStr(body.Size, cfg.DefaultSize)
	quality := orStr(body.Quality, cfg.DefaultQuality)
	model := orStr(body.Model, cfg.Model)

	var created []map[string]any
	for _, prompt := range prompts {
		for i := 0; i < repeat; i++ {
			t := tasks.Enqueue(uid, prompt, size, quality, model, fmtMode, n)
			created = append(created, map[string]any{"id": t.ID, "status": t.Status})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": created})
}

func listTasks(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	all := isAdmin(r) && r.URL.Query().Get("all") == "true"
	list := tasks.List(u.ID, all, 50)
	if all {
		for i := range list {
			if usr, ok := store.GetUser(list[i].UserID); ok {
				list[i].Username = usr.Username
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": list})
}

func getTask(w http.ResponseWriter, r *http.Request) {
	t, ok := tasks.Get(r.PathValue("id"))
	if !ok || (!isAdmin(r) && t.UserID != currentUser(r).ID) {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// clearTasks 清理已结束(done/error)的任务,保留排队中/生成中的。
func clearTasks(w http.ResponseWriter, r *http.Request) {
	all := isAdmin(r) && r.URL.Query().Get("all") == "true"
	removed := tasks.Clear(currentUser(r).ID, all)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed})
}

// safeOutputPath 把一个文件名安全地解析到「某用户」的图片目录内。
func safeOutputPath(userID int64, name string) string {
	base := filepath.Base(name)
	if base == "" || base == "." || base == "/" {
		return ""
	}
	dir := generator.UserDir(userID)
	p := filepath.Join(dir, base)
	abs, _ := filepath.Abs(p)
	dirAbs, _ := filepath.Abs(dir)
	if !strings.HasPrefix(abs, dirAbs) {
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// enforceImageLimit 按用户图片上限裁掉最旧历史并删盘上文件。
func enforceImageLimit(userID int64) {
	u, ok := store.GetUser(userID)
	if !ok || u.ImageLimit <= 0 {
		return
	}
	for _, f := range store.PruneUserImages(userID, u.ImageLimit) {
		_ = os.Remove(filepath.Join(generator.UserDir(userID), filepath.Base(f)))
	}
}

func editImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "解析上传失败:"+err.Error())
		return
	}
	prompt := r.FormValue("prompt")
	if strings.TrimSpace(prompt) == "" {
		writeErr(w, http.StatusBadRequest, "缺少 prompt")
		return
	}
	u := currentUser(r)
	cfg := config.LoadForUser(u.ID)
	var files []generator.EditFile
	if r.MultipartForm != nil {
		for _, fh := range r.MultipartForm.File["images"] {
			f, err := fh.Open()
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(f)
			f.Close()
			name := fh.Filename
			if name == "" {
				name = "image.png"
			}
			files = append(files, generator.EditFile{Name: name, Data: data})
		}
	}
	for _, fn := range strings.Split(r.FormValue("source"), ",") {
		fn = strings.TrimSpace(fn)
		if fn == "" {
			continue
		}
		if p := safeOutputPath(u.ID, fn); p != "" {
			if data, err := os.ReadFile(p); err == nil {
				files = append(files, generator.EditFile{Name: filepath.Base(p), Data: data})
			}
		}
	}
	if len(files) == 0 {
		writeErr(w, http.StatusBadRequest, "请上传至少一张图片(或选择一张已有结果继续)。")
		return
	}
	n := clamp(atoiDefault(r.FormValue("n"), 1), 1, 4)
	model := orStr(r.FormValue("model"), cfg.Model)
	size := orStr(r.FormValue("size"), cfg.DefaultSize)
	quality := orStr(r.FormValue("quality"), cfg.DefaultQuality)

	results, err := generator.Edit(r.Context(), prompt, files, generator.Params{
		UserID: u.ID, BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: model,
		Size: size, Quality: quality, N: n, Timeout: cfg.Timeout,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	fileNames := make([]string, 0, len(results))
	imgs := make([]map[string]any, 0, len(results))
	for _, r := range results {
		fileNames = append(fileNames, r.Filename)
		imgs = append(imgs, map[string]any{"filename": r.Filename, "url": r.URL})
	}
	store.AddHistory(u.ID, "edit", prompt, model, size, quality, len(results), fileNames)
	enforceImageLimit(u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"images": imgs})
}

// ----------------------------- 历史 / 收藏 -----------------------------

func listHistory(w http.ResponseWriter, r *http.Request) {
	limit := atoiDefault(r.URL.Query().Get("limit"), 100)
	if isAdmin(r) && r.URL.Query().Get("all") == "true" {
		writeJSON(w, http.StatusOK, map[string]any{"history": store.ListAllHistory(limit)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": store.ListHistory(currentUser(r).ID, limit)})
}

func deleteHistory(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	files, owner := store.DeleteHistory(id, currentUser(r).ID, isAdmin(r))
	if r.URL.Query().Get("with_files") == "true" && owner != 0 {
		for _, f := range files {
			_ = os.Remove(filepath.Join(generator.UserDir(owner), filepath.Base(f)))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// clearHistory 清空当前用户的历史记录;?with_files=true 时一并删除对应图片文件。
func clearHistory(w http.ResponseWriter, r *http.Request) {
	uid := currentUser(r).ID
	files := store.ClearHistory(uid)
	if r.URL.Query().Get("with_files") == "true" {
		for _, f := range files {
			_ = os.Remove(filepath.Join(generator.UserDir(uid), filepath.Base(f)))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func listFavorites(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"favorites": store.ListFavorites(currentUser(r).ID)})
}

func addFavorite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
		Name   string `json:"name"`
	}
	_ = decodeJSON(r, &body)
	if strings.TrimSpace(body.Prompt) == "" {
		writeErr(w, http.StatusBadRequest, "提示词为空")
		return
	}
	id := store.AddFavorite(currentUser(r).ID, strings.TrimSpace(body.Prompt), strings.TrimSpace(body.Name))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func deleteFavorite(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	store.DeleteFavorite(id, currentUser(r).ID, isAdmin(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------- 反推 -----------------------------

func reversePrompt(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "解析上传失败")
		return
	}
	f, _, err := r.FormFile("image")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "未收到图片")
		return
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if len(data) == 0 {
		writeErr(w, http.StatusBadRequest, "未收到图片")
		return
	}
	cfg := config.LoadForUser(currentUser(r).ID)
	model := orStr(r.FormValue("model"), orStr(cfg.ReverseModel, "gpt-4o"))
	text, err := generator.ReversePrompt(r.Context(), data, cfg.BaseURL, cfg.APIKey, model, cfg.Timeout)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompt": text})
}

// ----------------------------- 压测 -----------------------------

func stressStart(w http.ResponseWriter, r *http.Request) {
	if stress.State() == "running" {
		writeErr(w, http.StatusConflict, "已有压测在运行,请先停止或等待完成。")
		return
	}
	var body struct {
		Prompt        string `json:"prompt"`
		Total         int    `json:"total"`
		Concurrency   int    `json:"concurrency"`
		Model         string `json:"model"`
		Size          string `json:"size"`
		Quality       string `json:"quality"`
		RequestFormat string `json:"request_format"`
		Save          bool   `json:"save"`
	}
	_ = decodeJSON(r, &body)
	uid := currentUser(r).ID
	cfg := config.LoadForUser(uid)
	total := body.Total
	if total < 1 {
		total = 1
	}
	requested := body.Concurrency
	if requested < 1 {
		requested = 1
	}
	concurrency := requested
	capped := false
	if concurrency > stressMaxConcurrency {
		concurrency = stressMaxConcurrency
		capped = true
	}
	stress.Start(stress.StartParams{
		UserID:      uid,
		Prompt:      orStr(body.Prompt, "a cute cat"),
		Total:       total,
		Concurrency: concurrency,
		Model:       orStr(body.Model, cfg.Model),
		Size:        orStr(body.Size, "1024x1024"),
		Quality:     orStr(body.Quality, cfg.DefaultQuality),
		Fmt:         orStr(body.RequestFormat, cfg.RequestFormat),
		Save:        body.Save,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "total": total, "concurrency": concurrency,
		"requested": requested, "capped": capped, "cap": stressMaxConcurrency,
	})
}

func stressStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, stress.Stats())
}

func stressStop(w http.ResponseWriter, r *http.Request) {
	stress.Cancel()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------- 请求日志(200 无图) -----------------------------

// listRequestLogs 只返回轻量元信息(不含请求体/响应体)。普通用户仅看自己的;管理员加 ?all=true 看全部。
func listRequestLogs(w http.ResponseWriter, r *http.Request) {
	all := isAdmin(r) && r.URL.Query().Get("all") == "true"
	logs := reqlog.List(currentUser(r).ID, all, 0)
	if all {
		for i := range logs {
			if usr, ok := store.GetUser(logs[i].UserID); ok {
				logs[i].Username = usr.Username
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

// getRequestLog 按 id 返回单条完整记录(含请求体/响应体);校验归属。
func getRequestLog(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的日志 id")
		return
	}
	e, ok := reqlog.Get(id)
	if !ok || (!isAdmin(r) && e.UserID != currentUser(r).ID) {
		writeErr(w, http.StatusNotFound, "记录不存在(可能已被新日志挤出)")
		return
	}
	if usr, ok := store.GetUser(e.UserID); ok {
		e.Username = usr.Username
	}
	writeJSON(w, http.StatusOK, map[string]any{"log": e})
}

func clearRequestLogs(w http.ResponseWriter, r *http.Request) {
	all := isAdmin(r) && r.URL.Query().Get("all") == "true"
	reqlog.Clear(currentUser(r).ID, all)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------- 画廊 -----------------------------

func gallery(w http.ResponseWriter, r *http.Request) {
	uid := currentUser(r).ID
	entries, _ := os.ReadDir(generator.UserDir(uid))
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if len(names) > 60 {
		names = names[:60]
	}
	imgs := make([]map[string]any, 0, len(names))
	for _, n := range names {
		imgs = append(imgs, map[string]any{"filename": n, "url": "/outputs/" + strconv.FormatInt(uid, 10) + "/" + n})
	}
	writeJSON(w, http.StatusOK, map[string]any{"images": imgs})
}

// ----------------------------- 静态资源 -----------------------------

// serveOutput 提供某用户的图片;需登录,且只能访问自己的(管理员可访问任意)。
func serveOutput(w http.ResponseWriter, r *http.Request) {
	uid, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !isAdmin(r) && currentUser(r).ID != uid {
		http.NotFound(w, r)
		return
	}
	name := filepath.Base(r.PathValue("name"))
	dir := generator.UserDir(uid)
	p := filepath.Join(dir, name)
	abs, _ := filepath.Abs(p)
	dirAbs, _ := filepath.Abs(dir)
	if !strings.HasPrefix(abs, dirAbs) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}

func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(staticFS, p); err != nil {
			// SPA 回退:未知路径返回 index.html
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			w.Header().Set("Cache-Control", "no-cache")
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		_ = time.Now()
	})
}

// ----------------------------- 小工具 -----------------------------

func orStr(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func atoiDefault(s string, d int) int {
	if s == "" {
		return d
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return d
	}
	return n
}
