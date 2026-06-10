package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

var promptSources = map[string]string{
	"banana": "https://cdn.jsdelivr.net/gh/glidea/banana-prompt-quicker@main/prompts.json",
}

type marketEntry struct {
	Title              string   `json:"title"`
	Prompt             string   `json:"prompt"`
	Preview            string   `json:"preview"`
	Author             string   `json:"author"`
	Mode               string   `json:"mode"`
	Category           string   `json:"category"`
	ReferenceImageURLs []string `json:"reference_image_urls"`
}

type marketCacheItem struct {
	ts   time.Time
	data []marketEntry
}

var (
	marketMu    sync.Mutex
	marketCache = map[string]marketCacheItem{}
)

// promptMarket 拉取并缓存社区精选提示词(默认 banana-prompt-quicker)。
func promptMarket(w http.ResponseWriter, r *http.Request) {
	source := orStr(r.URL.Query().Get("source"), "banana")
	url, ok := promptSources[source]
	if !ok {
		writeErr(w, http.StatusBadRequest, "未知来源")
		return
	}
	marketMu.Lock()
	cached, hasCache := marketCache[url]
	marketMu.Unlock()
	if hasCache && time.Since(cached.ts) < time.Hour {
		writeJSON(w, http.StatusOK, map[string]any{"source": source, "prompts": cached.data, "cached": true})
		return
	}

	data, err := fetchMarket(r.Context(), url)
	if err != nil {
		if hasCache {
			writeJSON(w, http.StatusOK, map[string]any{"source": source, "prompts": cached.data, "cached": true, "stale": true})
			return
		}
		writeErr(w, http.StatusBadGateway, "拉取提示词市场失败:"+err.Error())
		return
	}
	marketMu.Lock()
	marketCache[url] = marketCacheItem{ts: time.Now(), data: data}
	marketMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"source": source, "prompts": data, "cached": false})
}

func fetchMarket(ctx context.Context, url string) ([]marketEntry, error) {
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errHTTP(resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var rawList []map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawList); err != nil {
		return nil, err
	}
	out := make([]marketEntry, 0, len(rawList))
	for _, x := range rawList {
		var e marketEntry
		_ = json.Unmarshal(x["title"], &e.Title)
		_ = json.Unmarshal(x["prompt"], &e.Prompt)
		if e.Prompt == "" {
			continue
		}
		_ = json.Unmarshal(x["preview"], &e.Preview)
		_ = json.Unmarshal(x["author"], &e.Author)
		_ = json.Unmarshal(x["mode"], &e.Mode)
		_ = json.Unmarshal(x["category"], &e.Category)
		_ = json.Unmarshal(x["reference_image_urls"], &e.ReferenceImageURLs)
		if e.Mode == "" {
			e.Mode = "generate"
		}
		out = append(out, e)
	}
	return out, nil
}

type errHTTP int

func (e errHTTP) Error() string { return "HTTP " + itoa(int(e)) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
