// Package reqlog 是一个进程内的环形日志,记录「HTTP 200 但没拿到图片」的异常,
// 方便事后排查中转站为什么返回成功却不带图。只存内存,进程重启即清空。
//
// 列表(List)只返回不含请求体/响应体的轻量元信息,避免一次性把大量响应体传给前端;
// 单条(Get)才返回完整的请求体/响应体(响应体不截断)。
package reqlog

import (
	"sync"
	"time"
)

// maxEntries 是环形缓冲容量。
const maxEntries = 1000

// Entry 是一条完整的「200 无图」记录(含请求体/响应体)。
type Entry struct {
	ID       int64  `json:"id"`
	Time     int64  `json:"time"`     // unix 秒
	Source   string `json:"source"`   // images | chat | edit | stress
	Endpoint string `json:"endpoint"` // 实际请求的中转站地址
	Model    string `json:"model"`
	Status   int    `json:"status"`   // 恒为 200(只记 200 无图)
	Reason   string `json:"reason"`   // 判定无图的原因
	Request  string `json:"request"`  // 请求体(完整)
	Response string `json:"response"` // 响应体(完整,不截断)
}

// Meta 是列表用的轻量元信息,不含请求体/响应体。
type Meta struct {
	ID       int64  `json:"id"`
	Time     int64  `json:"time"`
	Source   string `json:"source"`
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Status   int    `json:"status"`
	Reason   string `json:"reason"`
}

var (
	mu     sync.Mutex
	ring   [maxEntries]Entry
	size   int   // 有效条数(<= maxEntries)
	head   int   // 下一个写入位置
	nextID int64 // 自增 ID
)

// Add 记录一次「200 但未获取到图片」事件。请求体/响应体原样保存,不截断。
func Add(source, endpoint, model, request, response, reason string, status int) {
	mu.Lock()
	defer mu.Unlock()
	nextID++
	ring[head] = Entry{
		ID:       nextID,
		Time:     time.Now().Unix(),
		Source:   source,
		Endpoint: endpoint,
		Model:    model,
		Status:   status,
		Reason:   reason,
		Request:  request,
		Response: response,
	}
	head = (head + 1) % maxEntries
	if size < maxEntries {
		size++
	}
}

// List 返回最近的元信息,最新在前;limit<=0 表示全部。不含请求体/响应体。
func List(limit int) []Meta {
	mu.Lock()
	defer mu.Unlock()
	if limit <= 0 || limit > size {
		limit = size
	}
	out := make([]Meta, 0, limit)
	for i := 0; i < limit; i++ {
		e := ring[(head-1-i+maxEntries)%maxEntries]
		out = append(out, Meta{
			ID: e.ID, Time: e.Time, Source: e.Source, Endpoint: e.Endpoint,
			Model: e.Model, Status: e.Status, Reason: e.Reason,
		})
	}
	return out
}

// Get 按 ID 返回单条完整记录(含请求体/响应体)。第二个返回值表示是否找到。
func Get(id int64) (Entry, bool) {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < size; i++ {
		e := ring[(head-1-i+maxEntries)%maxEntries]
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Clear 清空全部日志。
func Clear() {
	mu.Lock()
	defer mu.Unlock()
	ring = [maxEntries]Entry{}
	size, head = 0, 0
}
