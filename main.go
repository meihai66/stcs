// stcs:GPT 和 Gemini 生图带压测——Go 重写版。
//
// 启动:STCS_PASSWORD=xxx ./stcs   (默认监听 :5311)
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/meihai66/stcs/internal/config"
	"github.com/meihai66/stcs/internal/generator"
	"github.com/meihai66/stcs/internal/httpapi"
	"github.com/meihai66/stcs/internal/store"
	"github.com/meihai66/stcs/internal/tasks"
	"github.com/meihai66/stcs/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	dataDir := env("STCS_DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("无法创建数据目录 %s: %v", dataDir, err)
	}

	// 初始化各子系统
	config.Init(filepath.Join(dataDir, "config.json"))
	generator.Init(filepath.Join(dataDir, "outputs"))
	if err := store.Init(filepath.Join(dataDir, "data.db")); err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	tasks.Start()

	addr := ":" + env("STCS_PORT", "5311")
	handler := httpapi.New(web.Dist())

	log.Printf("stcs 已启动:http://0.0.0.0%s  (数据目录 %s)", addr, dataDir)
	if os.Getenv("STCS_PASSWORD") == "" {
		log.Printf("⚠️  正在使用默认访问密码 admin888,生产环境请通过环境变量 STCS_PASSWORD 修改。")
	}
	srv := &http.Server{Addr: addr, Handler: handler}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
