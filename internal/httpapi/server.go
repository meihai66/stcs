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
	"github.com/meihai66/stcs/internal/store"
	"github.com/meihai66/stcs/internal/stress"
	"github.com/meihai66/stcs/internal/tasks"
)

const stressMaxConcurrency = 20000

// New 组装并返回完整的 http.Handler。staticFS 是已构建的前端 dist 文件系统。
func New(staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()

	// ---- 认证(不走密码门)----
	mux.HandleFunc("POST /api/login", handleLogin)
	mux.HandleFunc("POST /api/logout", handleLogout)
	mux.HandleFunc("GET /api/auth/status", handleAuthStatus)

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
	mux.HandleFunc("POST /api/edit", requireAuth(editImage))

	mux.HandleFunc("GET /api/history", requireAuth(listHistory))
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

	// ---- 对外 OpenAI 兼容 API(用 server_api_key 校验,不走密码门)----
	mux.HandleFunc("GET /v1/models", openaiModels)
	mux.HandleFunc("POST /v1/images/generations", openaiGenerations)

	// ---- 静态资源 ----
	mux.HandleFunc("GET /outputs/{name}", serveOutput) // 生成的图片(公开,供 <img> 与 API url 引用)
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
	cfg := config.Load()
	out := map[string]any{
		"base_url":            cfg.BaseURL,
		"model":               cfg.Model,
		"reverse_model":       cfg.ReverseModel,
		"request_format":      cfg.RequestFormat,
		"default_size":        cfg.DefaultSize,
		"default_quality":     cfg.DefaultQuality,
		"timeout":             cfg.Timeout,
		"concurrency":         cfg.Concurrency,
		"api_key":             maskKey(cfg.APIKey),
		"has_api_key":         cfg.APIKey != "",
		"server_api_key":      mask4(cfg.ServerAPIKey),
		"has_server_api_key":  cfg.ServerAPIKey != "",
		"running_workers":     tasks.RunningWorkers(),
		"active_profile":      cfg.ActiveProfile,
	}
	writeJSON(w, http.StatusOK, out)
}

func mask4(s string) string {
	if s == "" {
		return ""
	}
	return "****"
}

func setConfig(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := decodeJSON(r, &payload); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	updates := map[string]any{}
	for _, k := range []string{"default_size", "default_quality", "timeout", "concurrency"} {
		if v, ok := payload[k]; ok {
			updates[k] = v
		}
	}
	if v, ok := payload["server_api_key"].(string); ok && v != "" && !strings.HasPrefix(v, "****") {
		updates["server_api_key"] = v
	}
	cfg := config.SaveGlobal(updates)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "model": cfg.Model})
}

func listProfiles(w http.ResponseWriter, r *http.Request) {
	profs, active := config.ListProfiles()
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
	if err := config.SaveProfile(p); err != nil {
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
	config.SetActive(strings.TrimSpace(body.Name))
	cfg := config.Load()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active": cfg.ActiveProfile, "model": cfg.Model})
}

func deleteProfile(w http.ResponseWriter, r *http.Request) {
	config.DeleteProfile(r.PathValue("name"))
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
	cfg := config.Load()
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
			t := tasks.Enqueue(prompt, size, quality, model, fmtMode, n)
			created = append(created, map[string]any{"id": t.ID, "status": t.Status})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": created})
}

func listTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks.List(50)})
}

func getTask(w http.ResponseWriter, r *http.Request) {
	t, ok := tasks.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func safeOutputPath(name string) string {
	base := filepath.Base(name)
	if base == "" || base == "." || base == "/" {
		return ""
	}
	p := filepath.Join(generator.OutputDir(), base)
	abs, _ := filepath.Abs(p)
	outAbs, _ := filepath.Abs(generator.OutputDir())
	if !strings.HasPrefix(abs, outAbs) {
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
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
	cfg := config.Load()
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
		if p := safeOutputPath(fn); p != "" {
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
		BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: model,
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
	store.AddHistory("edit", prompt, model, size, quality, len(results), fileNames)
	writeJSON(w, http.StatusOK, map[string]any{"images": imgs})
}

// ----------------------------- 历史 / 收藏 -----------------------------

func listHistory(w http.ResponseWriter, r *http.Request) {
	limit := atoiDefault(r.URL.Query().Get("limit"), 100)
	writeJSON(w, http.StatusOK, map[string]any{"history": store.ListHistory(limit)})
}

func deleteHistory(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	files := store.DeleteHistory(id)
	if r.URL.Query().Get("with_files") == "true" {
		for _, f := range files {
			_ = os.Remove(filepath.Join(generator.OutputDir(), filepath.Base(f)))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func listFavorites(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"favorites": store.ListFavorites()})
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
	id := store.AddFavorite(strings.TrimSpace(body.Prompt), strings.TrimSpace(body.Name))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func deleteFavorite(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	store.DeleteFavorite(id)
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
	cfg := config.Load()
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
	}
	_ = decodeJSON(r, &body)
	cfg := config.Load()
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
		Prompt:      orStr(body.Prompt, "a cute cat"),
		Total:       total,
		Concurrency: concurrency,
		Model:       orStr(body.Model, cfg.Model),
		Size:        orStr(body.Size, "1024x1024"),
		Quality:     orStr(body.Quality, cfg.DefaultQuality),
		Fmt:         orStr(body.RequestFormat, cfg.RequestFormat),
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

// ----------------------------- 画廊 -----------------------------

func gallery(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(generator.OutputDir())
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
		imgs = append(imgs, map[string]any{"filename": n, "url": "/outputs/" + n})
	}
	writeJSON(w, http.StatusOK, map[string]any{"images": imgs})
}

// ----------------------------- 静态资源 -----------------------------

func serveOutput(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	p := filepath.Join(generator.OutputDir(), name)
	abs, _ := filepath.Abs(p)
	outAbs, _ := filepath.Abs(generator.OutputDir())
	if !strings.HasPrefix(abs, outAbs) {
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
