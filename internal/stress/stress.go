// Package stress 实现生图压测:按自定义并发向中转站发起 N 次真实生图请求,
// 统计成功率/延迟/吞吐。压测只测量、不保存图片。
//
// Go 原生并发,直接用 goroutine 工作池实现,无 Python 版的 select 512 限制问题。
package stress

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/meihai66/stcs/internal/config"
	"github.com/meihai66/stcs/internal/generator"
)

type run struct {
	mu          sync.Mutex
	Status      string // running|done|cancelled|error
	Total       int
	Concurrency int
	Model       string
	Size        string
	Fmt         string
	Done        int
	OK          int
	Fail        int
	Latencies   []float64
	Errors      map[string]int
	StartMono   time.Time
	StartedAt   int64
	Elapsed     float64
	ErrMsg      string
	cancel      context.CancelFunc
}

var (
	gmu     sync.Mutex
	current *run
	httpcli = &http.Client{}
)

// State 返回当前压测的简单状态(供启动前判断是否在跑)。
func State() string {
	gmu.Lock()
	defer gmu.Unlock()
	if current == nil {
		return "idle"
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	return current.Status
}

// StartParams 是一轮压测参数。
type StartParams struct {
	Prompt      string
	Total       int
	Concurrency int
	Model       string
	Size        string
	Quality     string
	Fmt         string
}

// Start 启动一轮压测(异步)。
func Start(sp StartParams) {
	cfg := config.Load()
	base, key := cfg.BaseURL, cfg.APIKey

	r := &run{
		Status: "running", Total: sp.Total, Concurrency: sp.Concurrency,
		Model: sp.Model, Size: sp.Size, Fmt: sp.Fmt,
		Errors: map[string]int{}, StartMono: time.Now(), StartedAt: time.Now().Unix(),
	}
	gmu.Lock()
	current = r
	gmu.Unlock()

	if base == "" || key == "" {
		r.Status = "error"
		r.ErrMsg = "未配置中转站地址或密钥"
		return
	}

	endpoint := strings.TrimRight(base, "/")
	if sp.Fmt == "chat" {
		endpoint += "/v1/chat/completions"
	} else {
		endpoint += "/v1/images/generations"
	}
	var payload map[string]any
	if sp.Fmt == "chat" {
		payload = map[string]any{
			"model":    sp.Model,
			"messages": []map[string]any{{"role": "user", "content": sp.Prompt}},
			"stream":   false,
		}
	} else {
		payload = map[string]any{"model": sp.Model, "prompt": sp.Prompt, "n": 1, "size": sp.Size}
		if sp.Quality != "" {
			payload["quality"] = sp.Quality
		}
	}
	body, _ := json.Marshal(payload)
	headers := map[string]string{"Authorization": "Bearer " + key, "Content-Type": "application/json"}
	timeout := time.Duration(cfg.Timeout) * time.Second

	conc := sp.Concurrency
	if conc > sp.Total {
		conc = sp.Total
	}
	r.Concurrency = conc

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	go func() {
		var dispatched int64
		var dmu sync.Mutex
		var wg sync.WaitGroup
		for i := 0; i < conc; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					if ctx.Err() != nil {
						return
					}
					dmu.Lock()
					if dispatched >= int64(sp.Total) {
						dmu.Unlock()
						return
					}
					dispatched++
					dmu.Unlock()

					ok, lat, status, errLine := doOne(ctx, endpoint, body, headers, timeout, sp.Fmt)
					r.mu.Lock()
					r.Done++
					r.Latencies = append(r.Latencies, lat)
					if ok {
						r.OK++
					} else {
						r.Fail++
						label := errLine
						if status != 0 {
							label = "[" + itoa(status) + "] " + errLine
						}
						r.Errors[label]++
					}
					r.mu.Unlock()
				}
			}()
		}
		wg.Wait()
		r.mu.Lock()
		if r.Status == "running" {
			if ctx.Err() != nil {
				r.Status = "cancelled"
			} else {
				r.Status = "done"
			}
		}
		r.Elapsed = time.Since(r.StartMono).Seconds()
		r.mu.Unlock()
	}()
}

func doOne(ctx context.Context, endpoint string, body []byte, headers map[string]string, timeout time.Duration, fmtMode string) (ok bool, latMs float64, status int, errLine string) {
	start := time.Now()
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return false, ms(start), 0, "构造请求失败"
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpcli.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ms(start), 0, "已取消"
		}
		if strings.Contains(err.Error(), "deadline") || strings.Contains(err.Error(), "timeout") {
			return false, ms(start), 0, "超时"
		}
		return false, ms(start), 0, "连接失败:" + err.Error()
	}
	defer resp.Body.Close()
	data := readAll(resp.Body)
	lat := ms(start)
	if resp.StatusCode != 200 {
		line := firstLine(generator.ExtractError(resp.StatusCode, data, resp.Header), 160)
		return false, lat, resp.StatusCode, line
	}
	if fmtMode != "chat" {
		var parsed struct {
			Data []json.RawMessage `json:"data"`
		}
		if json.Unmarshal(data, &parsed) != nil || len(parsed.Data) == 0 {
			return false, lat, 200, "200 但无图片数据"
		}
	}
	return true, lat, 200, ""
}

// Cancel 停止当前压测。
func Cancel() {
	gmu.Lock()
	r := current
	gmu.Unlock()
	if r == nil {
		return
	}
	r.mu.Lock()
	running := r.Status == "running"
	c := r.cancel
	r.mu.Unlock()
	if running && c != nil {
		c()
	}
}

// Stats 返回当前压测的统计数据。
func Stats() map[string]any {
	gmu.Lock()
	r := current
	gmu.Unlock()
	if r == nil {
		return map[string]any{"status": "idle"}
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	lats := append([]float64(nil), r.Latencies...)
	sort.Float64s(lats)
	errs := map[string]int{}
	for k, v := range r.Errors {
		errs[k] = v
	}
	elapsed := r.Elapsed
	if elapsed == 0 && !r.StartMono.IsZero() {
		elapsed = time.Since(r.StartMono).Seconds()
	}
	rps := 0.0
	if elapsed > 0 {
		rps = round(float64(r.Done)/elapsed, 2)
	}
	out := map[string]any{
		"status":      r.Status,
		"total":       r.Total,
		"concurrency": r.Concurrency,
		"model":       r.Model,
		"size":        r.Size,
		"fmt":         r.Fmt,
		"done":        r.Done,
		"ok":          r.OK,
		"fail":        r.Fail,
		"elapsed":     round(elapsed, 2),
		"rps":         rps,
		"lat_min":     pickMin(lats),
		"lat_max":     pickMax(lats),
		"lat_avg":     avg(lats),
		"lat_p50":     pct(lats, 50),
		"lat_p95":     pct(lats, 95),
		"errors":      errs,
	}
	if r.ErrMsg != "" {
		out["error"] = r.ErrMsg
	}
	return out
}

// ---- helpers ----

func ms(start time.Time) float64 { return float64(time.Since(start).Microseconds()) / 1000.0 }

func readAll(r interface{ Read([]byte) (int, error) }) []byte {
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
		if len(buf) > 1<<20 { // 压测只看是否有数据,读 1MB 足够
			break
		}
	}
	return buf
}

func firstLine(s string, n int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > n {
		s = s[:n]
	}
	return s
}

func pct(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * len(sorted) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return round(sorted[idx], 0)
}

func pickMin(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	return round(s[0], 0)
}
func pickMax(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	return round(s[len(s)-1], 0)
}
func avg(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	var sum float64
	for _, v := range s {
		sum += v
	}
	return round(sum/float64(len(s)), 0)
}

func round(v float64, dec int) float64 {
	pow := 1.0
	for i := 0; i < dec; i++ {
		pow *= 10
	}
	return float64(int64(v*pow+0.5)) / pow
}

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
