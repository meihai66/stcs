// Package reqlog 是一个进程内日志,记录「HTTP 200 但没拿到图片」的异常,按用户区分,
// 方便事后排查中转站为什么返回成功却不带图。只存内存,进程重启即清空。
//
// 列表(List)只返回不含请求体/响应体的轻量元信息;单条(Get)才返回完整体(响应体不截断)。
// 保存条数上限可由管理员动态调整(SetCap)。
package reqlog

import (
	"sync"
	"time"
)

// Entry 是一条完整的「200 无图」记录(含请求体/响应体)。
type Entry struct {
	ID       int64  `json:"id"`
	Time     int64  `json:"time"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username,omitempty"` // 仅管理员视角填充
	Source   string `json:"source"`             // images | chat | edit | stress
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Status   int    `json:"status"`
	Reason   string `json:"reason"`
	Request  string `json:"request"`
	Response string `json:"response"`
}

// Meta 是列表用的轻量元信息,不含请求体/响应体。
type Meta struct {
	ID       int64  `json:"id"`
	Time     int64  `json:"time"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username,omitempty"`
	Source   string `json:"source"`
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Status   int    `json:"status"`
	Reason   string `json:"reason"`
}

var (
	mu     sync.Mutex
	buf    []Entry // 旧 → 新
	maxN   = 1000
	nextID int64
)

// SetCap 设置最大保存条数(管理员)。
func SetCap(n int) {
	mu.Lock()
	defer mu.Unlock()
	if n > 0 {
		maxN = n
		trim()
	}
}

// Cap 返回当前上限。
func Cap() int {
	mu.Lock()
	defer mu.Unlock()
	return maxN
}

func trim() {
	if len(buf) > maxN {
		buf = append([]Entry(nil), buf[len(buf)-maxN:]...)
	}
}

// Add 记录一次「200 但未获取到图片」事件。请求体/响应体原样保存,不截断。
func Add(userID int64, source, endpoint, model, request, response, reason string, status int) {
	mu.Lock()
	defer mu.Unlock()
	nextID++
	buf = append(buf, Entry{
		ID: nextID, Time: time.Now().Unix(), UserID: userID,
		Source: source, Endpoint: endpoint, Model: model,
		Status: status, Reason: reason, Request: request, Response: response,
	})
	trim()
}

// List 返回最近的元信息,最新在前。all=true(管理员)返回全部用户,否则只返回 userID 的。
func List(userID int64, all bool, limit int) []Meta {
	mu.Lock()
	defer mu.Unlock()
	out := []Meta{}
	for i := len(buf) - 1; i >= 0; i-- {
		if limit > 0 && len(out) >= limit {
			break
		}
		e := buf[i]
		if !all && e.UserID != userID {
			continue
		}
		out = append(out, Meta{
			ID: e.ID, Time: e.Time, UserID: e.UserID, Source: e.Source,
			Endpoint: e.Endpoint, Model: e.Model, Status: e.Status, Reason: e.Reason,
		})
	}
	return out
}

// Get 按 id 返回单条完整记录。第二个返回值表示是否找到。
func Get(id int64) (Entry, bool) {
	mu.Lock()
	defer mu.Unlock()
	for i := len(buf) - 1; i >= 0; i-- {
		if buf[i].ID == id {
			return buf[i], true
		}
	}
	return Entry{}, false
}

// Clear 清空日志。all=true 清全部;否则只清某用户的。
func Clear(userID int64, all bool) {
	mu.Lock()
	defer mu.Unlock()
	if all {
		buf = nil
		return
	}
	kept := make([]Entry, 0, len(buf))
	for _, e := range buf {
		if e.UserID != userID {
			kept = append(kept, e)
		}
	}
	buf = kept
}
