package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestInitOnLegacyDB 模拟旧版本(history/favorites 无 user_id 列)的库,
// 验证 Init 能平滑升级:补列 + 建索引不报「no such column: user_id」。
func TestInitOnLegacyDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// 1) 造一个旧 schema 的库(无 user_id),塞一条历史。
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(`
		CREATE TABLE history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL, mode TEXT NOT NULL, prompt TEXT NOT NULL,
			model TEXT, size TEXT, quality TEXT, n INTEGER, files TEXT
		);
		CREATE TABLE favorites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL, name TEXT, prompt TEXT NOT NULL
		);
		INSERT INTO history (created_at, mode, prompt, model, size, quality, n, files)
			VALUES (1, 'gen', '旧图', 'm', 's', 'q', 1, '["x.png"]');`)
	if err != nil {
		t.Fatal(err)
	}
	old.Close()

	// 2) 用新版 Init 打开同一个库——修复前这里会因索引引用 user_id 而报错。
	if err := Init(path); err != nil {
		t.Fatalf("Init 旧库失败: %v", err)
	}

	// 3) 旧数据 user_id 默认 0,应能按 user_id=0 查到。
	h := ListHistory(0, 100)
	if len(h) != 1 || h[0].Prompt != "旧图" {
		t.Fatalf("旧历史未保留: %+v", h)
	}

	// 4) 归并到管理员(id=7),再确认迁移。
	ReassignLegacyData(7)
	if len(ListHistory(0, 100)) != 0 || len(ListHistory(7, 100)) != 1 {
		t.Fatal("ReassignLegacyData 迁移失败")
	}
}
