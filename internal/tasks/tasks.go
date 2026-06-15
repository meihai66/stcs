// Package tasks 实现异步任务队列:批量文生图任务排队、并发执行、可查询进度。
// 任务完成后写入历史(store.AddHistory)。图生图(带文件上传)由 server 直接同步调用,不走队列。
package tasks

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/meihai66/stcs/internal/config"
	"github.com/meihai66/stcs/internal/generator"
	"github.com/meihai66/stcs/internal/store"
)

const maxKeep = 100

// Task 是一个文生图任务。
type Task struct {
	ID            string           `json:"id"`
	UserID        int64            `json:"user_id"`
	Username      string           `json:"username,omitempty"` // 仅管理员「查看全部」时填充
	Mode          string           `json:"mode"`
	Status        string           `json:"status"` // queued|running|done|error
	Prompt        string           `json:"prompt"`
	Size          string           `json:"size"`
	Quality       string           `json:"quality"`
	N             int              `json:"n"`
	Model         string           `json:"model"`
	RequestFormat string           `json:"request_format"`
	CreatedAt     int64            `json:"created_at"`
	StartedAt     *int64           `json:"started_at"`
	FinishedAt    *int64           `json:"finished_at"`
	Images        []store.Image    `json:"images"`
	Error         string           `json:"error"`
}

var (
	mu       sync.Mutex
	tasks    = map[string]*Task{}
	order    []string // 保持插入顺序
	queue    chan string
	started  bool
	workers  int
)

// Start 初始化队列并按并发数启动 worker。幂等。
func Start() {
	mu.Lock()
	defer mu.Unlock()
	if started {
		return
	}
	c := config.GlobalConcurrency()
	if c < 1 {
		c = 1
	}
	if c > 16 {
		c = 16
	}
	workers = c
	queue = make(chan string, 4096)
	for i := 0; i < c; i++ {
		go worker()
	}
	started = true
}

// RunningWorkers 返回当前 worker 数。
func RunningWorkers() int {
	mu.Lock()
	defer mu.Unlock()
	return workers
}

func now() int64 { return time.Now().Unix() }

func randID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// Enqueue 加入一个文生图任务,返回任务快照。
func Enqueue(userID int64, prompt, size, quality, model, requestFormat string, n int) Task {
	mu.Lock()
	t := &Task{
		ID:            randID(),
		UserID:        userID,
		Mode:          "gen",
		Status:        "queued",
		Prompt:        prompt,
		Size:          size,
		Quality:       quality,
		N:             n,
		Model:         model,
		RequestFormat: requestFormat,
		CreatedAt:     now(),
		Images:        []store.Image{},
	}
	tasks[t.ID] = t
	order = append(order, t.ID)
	// 超出上限清理最旧的已结束任务
	for len(order) > maxKeep {
		removed := false
		for i, id := range order {
			if tt := tasks[id]; tt != nil && (tt.Status == "done" || tt.Status == "error") {
				delete(tasks, id)
				order = append(order[:i], order[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			break
		}
	}
	snap := *t
	mu.Unlock()
	queue <- t.ID
	return snap
}

// Get 返回一个任务快照。
func Get(id string) (Task, bool) {
	mu.Lock()
	defer mu.Unlock()
	t, ok := tasks[id]
	if !ok {
		return Task{}, false
	}
	return *t, true
}

// List 倒序返回最近的任务快照。all=true(管理员)返回全部用户,否则只返回 userID 的。
func List(userID int64, all bool, limit int) []Task {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Task, 0, limit)
	for i := len(order) - 1; i >= 0 && len(out) < limit; i-- {
		t := tasks[order[i]]
		if t == nil || (!all && t.UserID != userID) {
			continue
		}
		out = append(out, *t)
	}
	return out
}

// Clear 清理已结束(done/error)的任务,保留排队中/生成中的,返回清理数量。
// all=true(管理员)清全部用户,否则只清 userID 的。
func Clear(userID int64, all bool) int {
	mu.Lock()
	defer mu.Unlock()
	kept := make([]string, 0, len(order))
	removed := 0
	for _, id := range order {
		t := tasks[id]
		if t != nil && (all || t.UserID == userID) && (t.Status == "done" || t.Status == "error") {
			delete(tasks, id)
			removed++
			continue
		}
		kept = append(kept, id)
	}
	order = kept
	return removed
}

func worker() {
	for id := range queue {
		runOne(id)
	}
}

func setStatus(id, status, errMsg string, images []store.Image, started, finished bool) {
	mu.Lock()
	defer mu.Unlock()
	t := tasks[id]
	if t == nil {
		return
	}
	if status != "" {
		t.Status = status
	}
	if errMsg != "" {
		t.Error = errMsg
	}
	if images != nil {
		t.Images = images
	}
	if started {
		n := now()
		t.StartedAt = &n
	}
	if finished {
		n := now()
		t.FinishedAt = &n
	}
}

func runOne(id string) {
	mu.Lock()
	t := tasks[id]
	if t == nil {
		mu.Unlock()
		return
	}
	snap := *t
	mu.Unlock()

	setStatus(id, "running", "", nil, true, false)

	cfg := config.LoadForUser(snap.UserID)
	fmtMode := snap.RequestFormat
	if fmtMode == "" {
		fmtMode = cfg.RequestFormat
	}
	model := snap.Model
	if model == "" {
		model = cfg.Model
	}
	size := snap.Size
	if size == "" {
		size = cfg.DefaultSize
	}
	quality := snap.Quality
	if quality == "" {
		quality = cfg.DefaultQuality
	}
	p := generator.Params{
		UserID:  snap.UserID,
		Prompt:  snap.Prompt,
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   model,
		Size:    size,
		Quality: quality,
		N:       snap.N,
		Timeout: cfg.Timeout,
	}

	var results []generator.Result
	var err error
	if fmtMode == "chat" {
		results, err = generator.GenerateViaChat(context.Background(), p)
	} else {
		results, err = generator.Generate(context.Background(), p)
	}
	if err != nil {
		msg := err.Error()
		if !generator.IsGenError(err) {
			msg = "内部错误:" + msg
		}
		setStatus(id, "error", msg, nil, false, true)
		return
	}
	imgs := make([]store.Image, 0, len(results))
	files := make([]string, 0, len(results))
	for _, r := range results {
		imgs = append(imgs, store.Image{Filename: r.Filename, URL: r.URL})
		files = append(files, r.Filename)
	}
	setStatus(id, "done", "", imgs, false, true)
	store.AddHistory(snap.UserID, "gen", snap.Prompt, model, size, quality, snap.N, files)
	pruneUserImages(snap.UserID)
}

// pruneUserImages 按用户图片上限裁掉最旧历史,并从磁盘删除对应文件。
func pruneUserImages(userID int64) {
	u, ok := store.GetUser(userID)
	if !ok || u.ImageLimit <= 0 {
		return
	}
	for _, f := range store.PruneUserImages(userID, u.ImageLimit) {
		_ = os.Remove(filepath.Join(generator.UserDir(userID), filepath.Base(f)))
	}
}
