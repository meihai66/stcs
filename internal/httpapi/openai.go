package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/meihai66/stcs/internal/config"
	"github.com/meihai66/stcs/internal/generator"
	"github.com/meihai66/stcs/internal/store"
)

// checkServerKey 校验对外 API 的 server_api_key(未设置则不校验)。
func checkServerKey(r *http.Request) bool {
	required := config.Load().ServerAPIKey
	if required == "" {
		return true
	}
	token := strings.TrimSpace(strings.ReplaceAll(r.Header.Get("Authorization"), "Bearer ", ""))
	return token == required
}

func openaiModels(w http.ResponseWriter, r *http.Request) {
	if !checkServerKey(r) {
		writeErr(w, http.StatusUnauthorized, "无效的 API Key")
		return
	}
	cfg := config.Load()
	model := cfg.Model
	if model == "" {
		model = "gpt-image-2"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   []map[string]any{{"id": model, "object": "model", "owned_by": "local"}},
	})
}

func openaiGenerations(w http.ResponseWriter, r *http.Request) {
	if !checkServerKey(r) {
		writeErr(w, http.StatusUnauthorized, "无效的 API Key")
		return
	}
	var payload struct {
		Prompt         string `json:"prompt"`
		Model          string `json:"model"`
		Size           string `json:"size"`
		Quality        string `json:"quality"`
		N              int    `json:"n"`
		ResponseFormat string `json:"response_format"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	if strings.TrimSpace(payload.Prompt) == "" {
		writeErr(w, http.StatusBadRequest, "缺少 prompt")
		return
	}
	cfg := config.Load()
	n := payload.N
	if n < 1 {
		n = 1
	}
	model := orStr(payload.Model, cfg.Model)
	size := orStr(payload.Size, cfg.DefaultSize)
	quality := orStr(payload.Quality, cfg.DefaultQuality)
	results, err := generator.Generate(r.Context(), generator.Params{
		Prompt: payload.Prompt, BaseURL: cfg.BaseURL, APIKey: cfg.APIKey,
		Model: model, Size: size, Quality: quality, N: n, Timeout: cfg.Timeout,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	files := make([]string, 0, len(results))
	for _, r := range results {
		files = append(files, r.Filename)
	}
	store.AddHistory("api", payload.Prompt, model, size, quality, len(results), files)

	respFormat := orStr(payload.ResponseFormat, "b64_json")
	data := make([]map[string]any, 0, len(results))
	for _, r := range results {
		if respFormat == "url" {
			data = append(data, map[string]any{"url": r.URL})
		} else {
			data = append(data, map[string]any{"b64_json": r.B64JSON})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": time.Now().Unix(), "data": data})
}
