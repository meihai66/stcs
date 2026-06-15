// Package store 用 SQLite 持久化用户、会话、生图历史、提示词收藏与全局设置。
// 使用纯 Go 的 modernc.org/sqlite,无需 CGO,方便静态编译进 Docker。
//
// 多用户:history/favorites 均带 user_id,数据按用户隔离;管理员可跨用户查看。
package store

import (
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	db *sql.DB
	mu sync.Mutex
)

// Image 是历史里一张图的引用。
type Image struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

// History 是一条生图历史。
type History struct {
	ID        int64    `json:"id"`
	UserID    int64    `json:"user_id"`
	Username  string   `json:"username,omitempty"` // 仅管理员「查看全部」时填充
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
	UserID    int64  `json:"user_id"`
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
	// 1) 先建表(对旧库,IF NOT EXISTS 是空操作,不会改动已有 history/favorites)。
	if _, err = d.Exec(`
		CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			mode TEXT NOT NULL,
			prompt TEXT NOT NULL,
			model TEXT, size TEXT, quality TEXT, n INTEGER,
			files TEXT,
			user_id INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS favorites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			name TEXT, prompt TEXT NOT NULL,
			user_id INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			image_limit INTEGER NOT NULL DEFAULT 200,
			config TEXT,
			created_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);
	`); err != nil {
		return err
	}
	db = d
	// 2) 兼容旧库:补 user_id 列(必须在建索引之前,否则索引引用不存在的列会失败)。
	addColumnIfMissing("history", "user_id", "INTEGER NOT NULL DEFAULT 0")
	addColumnIfMissing("favorites", "user_id", "INTEGER NOT NULL DEFAULT 0")
	// 3) 列齐了再建索引。
	_, _ = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_history_user ON history(user_id);
		CREATE INDEX IF NOT EXISTS idx_favorites_user ON favorites(user_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_exp ON sessions(expires_at);
	`)
	return nil
}

func addColumnIfMissing(table, col, decl string) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk) == nil && name == col {
			return // 已存在
		}
	}
	_, _ = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + col + " " + decl)
}

// ----------------------------- 历史 -----------------------------

// AddHistory 写入一条历史,返回新行 id。
func AddHistory(userID int64, mode, prompt, model, size, quality string, n int, files []string) int64 {
	mu.Lock()
	defer mu.Unlock()
	fj, _ := json.Marshal(files)
	res, err := db.Exec(
		`INSERT INTO history (created_at, mode, prompt, model, size, quality, n, files, user_id) VALUES (?,?,?,?,?,?,?,?,?)`,
		time.Now().Unix(), mode, prompt, model, size, quality, n, string(fj), userID)
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}

// ListHistory 倒序返回某用户的历史(等价于第一页)。
func ListHistory(userID int64, limit int) []History {
	return ListHistoryPage(userID, limit, 0)
}

// ListHistoryPage 倒序分页返回某用户的历史。offset 为跳过的条数。
func ListHistoryPage(userID int64, limit, offset int) []History {
	mu.Lock()
	defer mu.Unlock()
	rows, err := db.Query(`SELECT h.id, h.user_id, '', h.created_at, h.mode, h.prompt, h.model, h.size, h.quality, h.n, h.files
		FROM history h WHERE h.user_id=? ORDER BY h.id DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	return scanHistory(rows, err)
}

// ListAllHistory 管理员视角:倒序返回全部用户历史(带用户名,等价于第一页)。
func ListAllHistory(limit int) []History {
	return ListAllHistoryPage(limit, 0)
}

// ListAllHistoryPage 管理员视角:倒序分页返回全部用户历史(带用户名)。
func ListAllHistoryPage(limit, offset int) []History {
	mu.Lock()
	defer mu.Unlock()
	rows, err := db.Query(`SELECT h.id, h.user_id, COALESCE(u.username,''), h.created_at, h.mode, h.prompt, h.model, h.size, h.quality, h.n, h.files
		FROM history h LEFT JOIN users u ON u.id=h.user_id ORDER BY h.id DESC LIMIT ? OFFSET ?`, limit, offset)
	return scanHistory(rows, err)
}

// CountHistory 返回某用户的历史总条数。
func CountHistory(userID int64) int {
	mu.Lock()
	defer mu.Unlock()
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM history WHERE user_id=?`, userID).Scan(&n)
	return n
}

// CountAllHistory 返回全部用户的历史总条数。
func CountAllHistory() int {
	mu.Lock()
	defer mu.Unlock()
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM history`).Scan(&n)
	return n
}

func scanHistory(rows *sql.Rows, err error) []History {
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []History{}
	for rows.Next() {
		var h History
		var filesJSON, model, size, quality, username sql.NullString
		if err := rows.Scan(&h.ID, &h.UserID, &username, &h.CreatedAt, &h.Mode, &h.Prompt, &model, &size, &quality, &h.N, &filesJSON); err != nil {
			continue
		}
		h.Model, h.Size, h.Quality, h.Username = model.String, size.String, quality.String, username.String
		h.Files = []string{}
		if filesJSON.Valid {
			_ = json.Unmarshal([]byte(filesJSON.String), &h.Files)
		}
		h.Images = make([]Image, 0, len(h.Files))
		for _, f := range h.Files {
			h.Images = append(h.Images, Image{Filename: f, URL: imageURL(h.UserID, f)})
		}
		out = append(out, h)
	}
	return out
}

// imageURL 拼出某用户某文件的访问地址(/outputs/<uid>/<file>)。
func imageURL(userID int64, file string) string {
	return "/outputs/" + itoa64(userID) + "/" + file
}

// DeleteHistory 删除一条历史(校验归属;管理员可删任意),返回它关联的文件名与所属用户。
func DeleteHistory(id, userID int64, isAdmin bool) (files []string, owner int64) {
	mu.Lock()
	defer mu.Unlock()
	var filesJSON sql.NullString
	if db.QueryRow(`SELECT user_id, files FROM history WHERE id=?`, id).Scan(&owner, &filesJSON) != nil {
		return nil, 0
	}
	if !isAdmin && owner != userID {
		return nil, 0 // 越权
	}
	if filesJSON.Valid {
		_ = json.Unmarshal([]byte(filesJSON.String), &files)
	}
	_, _ = db.Exec(`DELETE FROM history WHERE id=?`, id)
	return files, owner
}

// ClearHistory 清空某用户的历史,返回其关联文件名。
func ClearHistory(userID int64) []string {
	mu.Lock()
	defer mu.Unlock()
	all := collectFiles(`SELECT files FROM history WHERE user_id=?`, userID)
	_, _ = db.Exec(`DELETE FROM history WHERE user_id=?`, userID)
	return all
}

// PruneUserImages 把某用户的历史图片裁到 limit 张以内(删最旧的整条历史),
// 返回被删掉的文件名供调用方从磁盘清理。limit<=0 表示不限制。
func PruneUserImages(userID int64, limit int) []string {
	if limit <= 0 {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	rows, err := db.Query(`SELECT id, files FROM history WHERE user_id=? ORDER BY id ASC`, userID)
	if err != nil {
		return nil
	}
	type rec struct {
		id    int64
		files []string
	}
	var recs []rec
	total := 0
	for rows.Next() {
		var id int64
		var fj sql.NullString
		if rows.Scan(&id, &fj) != nil {
			continue
		}
		var files []string
		if fj.Valid {
			_ = json.Unmarshal([]byte(fj.String), &files)
		}
		recs = append(recs, rec{id, files})
		total += len(files)
	}
	rows.Close()
	var removed []string
	for _, rc := range recs {
		if total <= limit {
			break
		}
		_, _ = db.Exec(`DELETE FROM history WHERE id=?`, rc.id)
		removed = append(removed, rc.files...)
		total -= len(rc.files)
	}
	return removed
}

func collectFiles(query string, args ...any) []string {
	var all []string
	rows, err := db.Query(query, args...)
	if err != nil {
		return all
	}
	defer rows.Close()
	for rows.Next() {
		var fj sql.NullString
		if rows.Scan(&fj) == nil && fj.Valid {
			var files []string
			if json.Unmarshal([]byte(fj.String), &files) == nil {
				all = append(all, files...)
			}
		}
	}
	return all
}

// ----------------------------- 收藏 -----------------------------

// AddFavorite 新增收藏,返回 id。
func AddFavorite(userID int64, prompt, name string) int64 {
	mu.Lock()
	defer mu.Unlock()
	res, err := db.Exec(`INSERT INTO favorites (created_at, name, prompt, user_id) VALUES (?,?,?,?)`,
		time.Now().Unix(), name, prompt, userID)
	if err != nil {
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}

// ListFavorites 倒序返回某用户的收藏。
func ListFavorites(userID int64) []Favorite {
	mu.Lock()
	defer mu.Unlock()
	rows, err := db.Query(`SELECT id, user_id, created_at, name, prompt FROM favorites WHERE user_id=? ORDER BY id DESC`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []Favorite{}
	for rows.Next() {
		var f Favorite
		var name sql.NullString
		if err := rows.Scan(&f.ID, &f.UserID, &f.CreatedAt, &name, &f.Prompt); err != nil {
			continue
		}
		f.Name = name.String
		out = append(out, f)
	}
	return out
}

// DeleteFavorite 删除一条收藏(校验归属;管理员可删任意)。
func DeleteFavorite(id, userID int64, isAdmin bool) {
	mu.Lock()
	defer mu.Unlock()
	if isAdmin {
		_, _ = db.Exec(`DELETE FROM favorites WHERE id=?`, id)
		return
	}
	_, _ = db.Exec(`DELETE FROM favorites WHERE id=? AND user_id=?`, id, userID)
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	pos := len(b)
	for v > 0 {
		pos--
		b[pos] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
