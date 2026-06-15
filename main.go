// stcs:GPT 和 Gemini 生图带压测——Go 重写版(多用户)。
//
// 启动:./stcs   (默认监听 :5311)
// 首个管理员:STCS_ADMIN_USER / STCS_ADMIN_PASSWORD(默认 admin / admin888)。
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/meihai66/stcs/internal/auth"
	"github.com/meihai66/stcs/internal/generator"
	"github.com/meihai66/stcs/internal/httpapi"
	"github.com/meihai66/stcs/internal/reqlog"
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

// migrateLegacyImages 把 outputs/ 根目录下平铺的 *.png 移入 outputs/<adminID>/。
func migrateLegacyImages(outputsDir string, adminID int64) {
	if adminID == 0 {
		return
	}
	entries, err := os.ReadDir(outputsDir)
	if err != nil {
		return
	}
	dst := filepath.Join(outputsDir, strconv.FormatInt(adminID, 10))
	moved := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
			continue
		}
		if moved == 0 {
			_ = os.MkdirAll(dst, 0o755)
		}
		if os.Rename(filepath.Join(outputsDir, e.Name()), filepath.Join(dst, e.Name())) == nil {
			moved++
		}
	}
	if moved > 0 {
		log.Printf("📦 已将 %d 张旧图片迁移到管理员目录。", moved)
	}
}

func main() {
	dataDir := env("STCS_DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("无法创建数据目录 %s: %v", dataDir, err)
	}

	// 初始化各子系统
	generator.Init(filepath.Join(dataDir, "outputs"))
	if err := store.Init(filepath.Join(dataDir, "data.db")); err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}

	// 首次运行:引导首个管理员,并把旧的全局 config.json + 历史数据归入它。
	legacyConfig := ""
	if b, err := os.ReadFile(filepath.Join(dataDir, "config.json")); err == nil {
		legacyConfig = string(b)
	}
	if created, name := auth.Bootstrap(legacyConfig); created {
		log.Printf("✅ 已创建首个管理员账户「%s」(密码见 STCS_ADMIN_PASSWORD / STCS_PASSWORD,默认 admin888,请尽快登录修改)。", name)
		// 把旧版平铺在 outputs/ 根目录的图片搬进管理员的子目录,保证历史里的图仍可访问。
		migrateLegacyImages(filepath.Join(dataDir, "outputs"), store.FirstAdminID())
	}
	store.PurgeExpiredSessions()
	if n, _ := strconv.Atoi(store.GetSetting("log_limit", "")); n > 0 {
		reqlog.SetCap(n)
	}

	tasks.Start()

	addr := ":" + env("STCS_PORT", "5311")
	handler := httpapi.New(web.Dist())

	log.Printf("stcs 已启动:http://0.0.0.0%s  (数据目录 %s)", addr, dataDir)
	srv := &http.Server{Addr: addr, Handler: handler}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
