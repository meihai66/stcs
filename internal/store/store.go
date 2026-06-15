// Package store 用 SQLite 持久化生图历史 + 提示词收藏。
// 使用纯 Go 的 modernc.org/sqlite,无需 CGO,方便静态编译进 Docker。
package store

import (
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	mu   sync.Mutex
)

// Image 是历史里一张图的引用。
type Image struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

// History 是一条生图历史。
type History struct {
	ID        int64    `json:"id"`
	CreatedAt int64    `json:"created_at"`
	Mode      string   `json:"mode"`
	Prompt    string   `json:"prompt"`
	Model     string   `json:"model"`
	Size      string   `json:"size"`
	Quality   string   `json:"quality"`
	N         int      `json:"n"`
	Files     []string `json:"files"`
	Images    []Image  `json:"images"`
}

// Favorite 是一条收藏的提示词。
type Favorite struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	CreatedAt int64  `json:"created_at"`
}

// Init 打开数据库并建表。
func Init(dbPath string) error {
	d, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	d.SetMaxOpenConns(1) // SQLite 单写,串行化避免锁冲突
	_, err = d.Exec(`
		CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			mode TEXT NOT NULL,
			prompt TEXT NOT NULL,
			model TEXT, size TEXT, quality TEXT, n INTEGER,
			files TEXT
		);
		CREATE TABLE IF NOT EXISTS favorites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			name TEXT, prompt TEXT NOT NULL
		);`)
	if err != nil {
		return err
	}
	db = d
	return nil
}

// AddHistory 写入一条历史,返回新行 id。
func AddHistory(mode, prompt, model, size, quality string, n int, files []string) int64 {
	mu.Lock()
	defer mu.Unlock()
	fj, _ := json.Marshal(files)
	res, err := db.Exec(
		`INSERT INTO history (created_at, mode, prompt, model, size, quality, n, files) VALUES (?,?,?,?,?,?,?,?)`,
		time.Now().Unix(), mode, prompt, model, size, quality, n, string(fj))
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}

// ListHistory 倒序返回历史。
func ListHistory(limit int) []History {
	mu.Lock()
	defer mu.Unlock()
	rows, err := db.Query(`SELECT id, created_at, mode, prompt, model, size, quality, n, files FROM history ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []History{}
	for rows.Next() {
		var h History
		var filesJSON sql.NullString
		var model, size, quality sql.NullString
		if err := rows.Scan(&h.ID, &h.CreatedAt, &h.Mode, &h.Prompt, &model, &size, &quality, &h.N, &filesJSON); err != nil {
			continue
		}
		h.Model, h.Size, h.Quality = model.String, size.String, quality.String
		h.Files = []string{}
		if filesJSON.Valid {
			_ = json.Unmarshal([]byte(filesJSON.String), &h.Files)
		}
		h.Images = make([]Image, 0, len(h.Files))
		for _, f := range h.Files {
			h.Images = append(h.Images, Image{Filename: f, URL: "/outputs/" + f})
		}
		out = append(out, h)
	}
	return out
}

// DeleteHistory 删除一条历史,返回它关联的文件名(供调用方决定是否删盘上文件)。
func DeleteHistory(id int64) []string {
	mu.Lock()
	defer mu.Unlock()
	var filesJSON sql.NullString
	_ = db.QueryRow(`SELECT files FROM history WHERE id=?`, id).Scan(&filesJSON)
	var files []string
	if filesJSON.Valid {
		_ = json.Unmarshal([]byte(filesJSON.String), &files)
	}
	_, _ = db.Exec(`DELETE FROM history WHERE id=?`, id)
	return files
}

// ClearHistory 清空全部历史,返回所有关联文件名(供调用方决定是否删盘上文件)。
func ClearHistory() []string {
	mu.Lock()
	defer mu.Unlock()
	var all []string
	if rows, err := db.Query(`SELECT files FROM history`); err == nil {
		for rows.Next() {
			var filesJSON sql.NullString
			if rows.Scan(&filesJSON) == nil && filesJSON.Valid {
				var files []string
				if json.Unmarshal([]byte(filesJSON.String), &files) == nil {
					all = append(all, files...)
				}
			}
		}
		rows.Close()
	}
	_, _ = db.Exec(`DELETE FROM history`)
	return all
}

// AddFavorite 新增收藏,返回 id。
func AddFavorite(prompt, name string) int64 {
	mu.Lock()
	defer mu.Unlock()
	res, err := db.Exec(`INSERT INTO favorites (created_at, name, prompt) VALUES (?,?,?)`, time.Now().Unix(), name, prompt)
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}

// ListFavorites 倒序返回收藏。
func ListFavorites() []Favorite {
	mu.Lock()
	defer mu.Unlock()
	rows, err := db.Query(`SELECT id, created_at, name, prompt FROM favorites ORDER BY id DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Favorite{}
	for rows.Next() {
		var f Favorite
		var name sql.NullString
		if err := rows.Scan(&f.ID, &f.CreatedAt, &name, &f.Prompt); err != nil {
			continue
		}
		f.Name = name.String
		out = append(out, f)
	}
	return out
}

// DeleteFavorite 删除一条收藏。
func DeleteFavorite(id int64) {
	mu.Lock()
	defer mu.Unlock()
	_, _ = db.Exec(`DELETE FROM favorites WHERE id=?`, id)
}
