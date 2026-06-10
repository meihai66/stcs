// Package generator 实现核心生图逻辑:调用中转站的 OpenAI 兼容接口,保存结果到 outputs/。
//
// 支持三种请求格式(中转站对 gpt-image 的实现各不相同):
//   - images: /v1/images/generations  标准图片接口
//   - chat:   /v1/chat/completions    对话式生图(图以 markdown / base64 返回)
//   - edits:  /v1/images/edits         图片编辑(需上传参考图)
package generator

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GenError 是带可读信息的生图错误。
type GenError struct{ Msg string }

func (e *GenError) Error() string { return e.Msg }

func genErr(format string, a ...any) error { return &GenError{Msg: fmt.Sprintf(format, a...)} }

// Result 是一张生成结果。
type Result struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
	B64JSON  string `json:"b64_json"`
}

var (
	outputDir string
	client    = &http.Client{}
)

// Init 设定图片输出目录。
func Init(dir string) {
	outputDir = dir
	_ = os.MkdirAll(outputDir, 0o755)
}

// OutputDir 返回输出目录。
func OutputDir() string { return outputDir }

var (
	retryCodes = map[int]bool{429: true, 500: true, 502: true, 503: true, 504: true}
	busyWords  = []string{"繁忙", "busy", "try again", "rate limit", "overloaded", "稍后"}
)

func isTransient(status int, body string) bool {
	if retryCodes[status] {
		return true
	}
	if status == 403 {
		low := strings.ToLower(body)
		for _, w := range busyWords {
			if strings.Contains(low, strings.ToLower(w)) || strings.Contains(body, w) {
				return true
			}
		}
	}
	return false
}

// postJSONRetry 发 JSON 请求,遇瞬时错误(繁忙/限流/网关)自动重试,带退避。
// 返回 status, body, err。
func postJSONRetry(ctx context.Context, endpoint string, payload any, headers map[string]string, timeout time.Duration, retries int) (int, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	var lastStatus int
	var lastBody []byte
	for attempt := 0; attempt <= retries; attempt++ {
		status, body, err := doPost(ctx, endpoint, "application/json", bytes.NewReader(raw), headers, timeout)
		if err != nil {
			return 0, nil, err
		}
		lastStatus, lastBody = status, body
		if status == 200 || !isTransient(status, string(body)) || attempt == retries {
			return status, body, nil
		}
		select {
		case <-ctx.Done():
			return status, body, ctx.Err()
		case <-time.After(time.Duration(1500*(attempt+1)) * time.Millisecond):
		}
	}
	return lastStatus, lastBody, nil
}

func doPost(ctx context.Context, endpoint, contentType string, body io.Reader, headers map[string]string, timeout time.Duration) (int, []byte, error) {
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, endpoint, body)
	if err != nil {
		return 0, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline") {
			return 0, nil, genErr("请求超时(%ds)。高分辨率可在设置里调大超时。", int(timeout.Seconds()))
		}
		return 0, nil, genErr("连接中转站失败:%v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data, nil
}

func savePNG(data []byte) (string, error) {
	name := fmt.Sprintf("%s-%s.png", time.Now().Format("20060102-150405"), randHex(4))
	if err := os.WriteFile(filepath.Join(outputDir, name), data, 0o644); err != nil {
		return "", err
	}
	return name, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// item 是接口返回 data 数组里的一项。
type genItem struct {
	B64JSON string `json:"b64_json"`
	URL     string `json:"url"`
}

func normalizeItems(ctx context.Context, items []genItem) ([]Result, error) {
	var out []Result
	for _, it := range items {
		var raw []byte
		if it.B64JSON != "" {
			d, err := base64.StdEncoding.DecodeString(it.B64JSON)
			if err != nil {
				continue
			}
			raw = d
		} else if it.URL != "" {
			d, err := fetchImage(ctx, it.URL)
			if err != nil || d == nil {
				continue
			}
			raw = d
		}
		if raw == nil {
			continue
		}
		name, err := savePNG(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, Result{
			Filename: name,
			URL:      "/outputs/" + name,
			B64JSON:  base64.StdEncoding.EncodeToString(raw),
		})
	}
	return out, nil
}

func fetchImage(ctx context.Context, u string) ([]byte, error) {
	if strings.HasPrefix(u, "data:") {
		idx := strings.Index(u, ",")
		if idx < 0 {
			return nil, nil
		}
		return base64.StdEncoding.DecodeString(u[idx+1:])
	}
	rctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Params 是一次文生图/对话生图的参数。
type Params struct {
	Prompt   string
	BaseURL  string
	APIKey   string
	Model    string
	Size     string
	Quality  string
	N        int
	Timeout  int
}

func (p Params) headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + p.APIKey}
}

// Generate 文生图(/v1/images/generations)。
func Generate(ctx context.Context, p Params) ([]Result, error) {
	if p.BaseURL == "" || p.APIKey == "" {
		return nil, genErr("未配置中转站地址或密钥,请先在「设置」中填写。")
	}
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/v1/images/generations"
	payload := map[string]any{"model": p.Model, "prompt": p.Prompt, "n": p.N, "size": p.Size}
	if p.Quality != "" {
		payload["quality"] = p.Quality
	}
	timeout := time.Duration(p.Timeout) * time.Second
	status, body, err := postJSONRetry(ctx, endpoint, payload, p.headers(), timeout, 2)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, genErr("%s", extractError(status, body, nil))
	}
	var parsed struct {
		Data []genItem `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, genErr("中转站返回非 JSON:%s", trunc(string(body), 300))
	}
	if len(parsed.Data) == 0 {
		return nil, genErr("中转站未返回图片数据:%s", trunc(string(body), 300))
	}
	results, err := normalizeItems(ctx, parsed.Data)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, genErr("返回的数据里没有可解析的图片(无 b64_json / url)。")
	}
	return results, nil
}

var (
	reMarkdownImg = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)
	reDataURI     = regexp.MustCompile(`(data:image/[^;]+;base64,[A-Za-z0-9+/=]+)`)
	reBareLink    = regexp.MustCompile(`(?i)(https?://[^\s)"']+\.(?:png|jpe?g|webp|gif))`)
)

func urlsFromContent(content json.RawMessage) []string {
	var out []string
	// 先试字符串
	var s string
	if json.Unmarshal(content, &s) == nil {
		for _, m := range reMarkdownImg.FindAllStringSubmatch(s, -1) {
			out = append(out, m[1])
		}
		for _, m := range reDataURI.FindAllString(s, -1) {
			out = append(out, m)
		}
		for _, m := range reBareLink.FindAllString(s, -1) {
			out = append(out, m)
		}
		return out
	}
	// 再试多模态数组
	var parts []map[string]json.RawMessage
	if json.Unmarshal(content, &parts) == nil {
		for _, part := range parts {
			var typ string
			_ = json.Unmarshal(part["type"], &typ)
			switch typ {
			case "image_url":
				out = append(out, extractImageURL(part["image_url"])...)
			case "text":
				var t string
				_ = json.Unmarshal(part["text"], &t)
				tb, _ := json.Marshal(t)
				out = append(out, urlsFromContent(tb)...)
			}
		}
	}
	return out
}

func extractImageURL(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var obj struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.URL != "" {
		return []string{obj.URL}
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return []string{s}
	}
	return nil
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage   `json:"content"`
			Images  []json.RawMessage `json:"images"`
		} `json:"message"`
	} `json:"choices"`
}

func extractImagesFromChat(ctx context.Context, body []byte) ([]Result, error) {
	var cr chatResp
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, nil
	}
	var urls []string
	for _, ch := range cr.Choices {
		urls = append(urls, urlsFromContent(ch.Message.Content)...)
		for _, im := range ch.Message.Images {
			var obj struct {
				ImageURL json.RawMessage `json:"image_url"`
				URL      string          `json:"url"`
			}
			if json.Unmarshal(im, &obj) == nil {
				if u := extractImageURL(obj.ImageURL); len(u) > 0 {
					urls = append(urls, u...)
				} else if obj.URL != "" {
					urls = append(urls, obj.URL)
				}
				continue
			}
			var s string
			if json.Unmarshal(im, &s) == nil && s != "" {
				urls = append(urls, s)
			}
		}
	}
	// 去重保序
	seen := map[string]bool{}
	var ordered []string
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			ordered = append(ordered, u)
		}
	}
	var out []Result
	for _, u := range ordered {
		raw, err := fetchImage(ctx, u)
		if err != nil || raw == nil {
			continue
		}
		name, err := savePNG(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, Result{Filename: name, URL: "/outputs/" + name, B64JSON: base64.StdEncoding.EncodeToString(raw)})
	}
	return out, nil
}

func chatText(body []byte) string {
	var cr struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(body, &cr) == nil && len(cr.Choices) > 0 {
		var s string
		if json.Unmarshal(cr.Choices[0].Message.Content, &s) == nil {
			return s
		}
		return string(cr.Choices[0].Message.Content)
	}
	return string(body)
}

// GenerateViaChat 用 /v1/chat/completions 生图,解析返回内容里的图片。
func GenerateViaChat(ctx context.Context, p Params) ([]Result, error) {
	if p.BaseURL == "" || p.APIKey == "" {
		return nil, genErr("未配置中转站地址或密钥,请先在「设置」中填写。")
	}
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/v1/chat/completions"
	headers := map[string]string{"Authorization": "Bearer " + p.APIKey}
	userPrompt := p.Prompt
	if p.Size != "" && p.Size != "auto" {
		userPrompt = fmt.Sprintf("%s\n\n(图片尺寸 image size: %s)", p.Prompt, p.Size)
	}
	timeout := time.Duration(p.Timeout) * time.Second
	n := p.N
	if n < 1 {
		n = 1
	}
	var results []Result
	for i := 0; i < n; i++ {
		payload := map[string]any{
			"model":    p.Model,
			"messages": []map[string]any{{"role": "user", "content": userPrompt}},
			"stream":   false,
		}
		status, body, err := postJSONRetry(ctx, endpoint, payload, headers, timeout, 2)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, genErr("%s", extractError(status, body, nil))
		}
		imgs, err := extractImagesFromChat(ctx, body)
		if err != nil {
			return nil, err
		}
		if len(imgs) == 0 {
			return nil, genErr("对话接口未返回图片(可能该模型/中转站不支持对话生图)。返回:%s", trunc(chatText(body), 200))
		}
		results = append(results, imgs...)
	}
	return results, nil
}

// EditFile 是一张待编辑/参考的图片。
type EditFile struct {
	Name string
	Data []byte
}

// Edit 图生图 / 编辑(/v1/images/edits)。
func Edit(ctx context.Context, prompt string, images []EditFile, p Params) ([]Result, error) {
	if p.BaseURL == "" || p.APIKey == "" {
		return nil, genErr("未配置中转站地址或密钥,请先在「设置」中填写。")
	}
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/v1/images/edits"

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", p.Model)
	_ = w.WriteField("prompt", prompt)
	_ = w.WriteField("n", strconv.Itoa(p.N))
	_ = w.WriteField("size", p.Size)
	if p.Quality != "" {
		_ = w.WriteField("quality", p.Quality)
	}
	for _, im := range images {
		fw, err := w.CreateFormFile("image", im.Name)
		if err != nil {
			return nil, err
		}
		_, _ = fw.Write(im.Data)
	}
	_ = w.Close()

	headers := map[string]string{"Authorization": "Bearer " + p.APIKey}
	timeout := time.Duration(p.Timeout) * time.Second
	status, body, err := doPost(ctx, endpoint, w.FormDataContentType(), &buf, headers, timeout)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, genErr("%s", extractError(status, body, nil))
	}
	var parsed struct {
		Data []genItem `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, genErr("中转站返回非 JSON:%s", trunc(string(body), 300))
	}
	results, err := normalizeItems(ctx, parsed.Data)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, genErr("编辑接口未返回可解析的图片。")
	}
	return results, nil
}

// ReversePrompt 参考图反推:把图片喂给视觉模型,返回可直接拿去生图的提示词。
func ReversePrompt(ctx context.Context, image []byte, baseURL, apiKey, model string, timeoutSec int) (string, error) {
	if baseURL == "" || apiKey == "" {
		return "", genErr("未配置中转站地址或密钥,请先在「设置」中填写。")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
	instr := "请仔细观察这张图片,用一段详细、可直接用于 AI 生图的提示词来描述它,涵盖主体、风格、构图、配色、光影、材质、氛围等。只输出提示词本身,不要解释、不要前后缀。"
	payload := map[string]any{
		"model": model,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": instr},
				{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
			},
		}},
		"stream": false,
	}
	headers := map[string]string{"Authorization": "Bearer " + apiKey}
	status, body, err := postJSONRetry(ctx, endpoint, payload, headers, time.Duration(timeoutSec)*time.Second, 2)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", genErr("%s", extractError(status, body, nil))
	}
	text := strings.TrimSpace(chatText(body))
	if text == "" {
		return "", genErr("反推失败:模型没有返回文本(该模型可能不支持看图)。")
	}
	return text, nil
}

var hints = map[int]string{
	401: "密钥无效或未授权,检查 API 密钥是否正确。",
	403: "被拒绝/禁止。常见原因:① 中转站上游繁忙或限流(稍后重试);② 该尺寸不被支持(先用 1024x1024 试);③ 密钥无此模型权限;④ 触发风控。",
	404: "接口不存在。可能该中转站不支持此请求格式,换「对话生图(chat)」或确认 base_url 是否正确(不要带 /v1)。",
	429: "触发限流(请求太频繁/超额)。把并发数调到 1、稍后再试,或检查额度。",
	400: "请求参数有误。最可能是尺寸不被支持(很多 gpt-image 只支持 1024x1024 / 1536x1024 / 1024x1536)。",
	500: "中转站上游内部错误,通常稍后重试可恢复。",
	502: "网关错误,中转站到上游的连接出问题,稍后重试。",
	503: "服务不可用,上游繁忙,稍后重试。",
	504: "网关超时,上游响应太慢,稍后重试或调大超时。",
}

// ExtractError 导出版本(供压测复用)。headers 可为 nil。
func ExtractError(status int, body []byte, hdr http.Header) string {
	return extractError(status, body, hdr)
}

func extractError(status int, body []byte, hdr http.Header) string {
	rid := ""
	if hdr != nil {
		rid = hdr.Get("x-request-id")
		if rid == "" {
			rid = hdr.Get("cf-ray")
		}
	}
	detail := ""
	var asObj map[string]json.RawMessage
	if json.Unmarshal(body, &asObj) == nil {
		errPart := asObj
		if e, ok := asObj["error"]; ok {
			var inner map[string]json.RawMessage
			if json.Unmarshal(e, &inner) == nil {
				errPart = inner
			}
		}
		var parts []string
		for _, k := range []string{"message", "type", "code", "param"} {
			if v, ok := errPart[k]; ok {
				var s string
				if json.Unmarshal(v, &s) == nil && s != "" {
					parts = append(parts, k+"="+s)
				} else if len(v) > 0 && string(v) != "null" {
					parts = append(parts, k+"="+string(v))
				}
			}
		}
		if len(parts) > 0 {
			detail = strings.Join(parts, " | ")
		}
	}
	if detail == "" {
		detail = trunc(string(body), 400)
		if detail == "" {
			detail = "(空响应体)"
		}
	}
	msg := fmt.Sprintf("[HTTP %d] %s", status, detail)
	if rid != "" {
		msg += "\nrequest-id: " + rid
	}
	if h, ok := hints[status]; ok {
		msg += "\n💡 " + h
	}
	return msg
}

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// IsGenError 判断是否为可读的生图错误。
func IsGenError(err error) bool {
	var ge *GenError
	return errors.As(err, &ge)
}
