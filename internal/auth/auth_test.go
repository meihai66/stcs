package auth

import (
	"path/filepath"
	"testing"

	"github.com/meihai66/stcs/internal/config"
	"github.com/meihai66/stcs/internal/store"
)

func TestAuthAndConfigFlow(t *testing.T) {
	dir := t.TempDir()
	if err := store.Init(filepath.Join(dir, "t.db")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STCS_ADMIN_USER", "root")
	t.Setenv("STCS_ADMIN_PASSWORD", "pw123456")

	// 引导首个管理员 + 迁移旧配置
	created, name := Bootstrap(`{"profiles":[{"name":"x","base_url":"http://h","api_key":"k","model":"m"}],"active_profile":"x"}`)
	if !created || name != "root" {
		t.Fatalf("bootstrap failed: created=%v name=%q", created, name)
	}
	if c, _ := Bootstrap(""); c {
		t.Fatal("bootstrap 应当幂等")
	}

	u, hash, ok := store.GetUserByUsername("root")
	if !ok || u.Role != "admin" {
		t.Fatalf("admin 未创建: %+v ok=%v", u, ok)
	}
	if !CheckPassword(hash, "pw123456") {
		t.Fatal("正确密码校验失败")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("错误密码竟通过")
	}

	// 迁移的配置归入 root
	cfg := config.LoadForUser(u.ID)
	if cfg.BaseURL != "http://h" || cfg.Model != "m" {
		t.Fatalf("旧配置未迁移: %+v", cfg)
	}

	// 会话
	tok, _ := CreateSession(u.ID)
	su, ok := store.SessionUser(tok)
	if !ok || su.ID != u.ID {
		t.Fatal("会话校验失败")
	}
	store.DeleteSession(tok)
	if _, ok := store.SessionUser(tok); ok {
		t.Fatal("会话删除后仍有效")
	}

	// 普通用户数据隔离
	uhash, _ := HashPassword("u1pass")
	uid, err := store.CreateUser("alice", uhash, "user", 5)
	if err != nil {
		t.Fatal(err)
	}
	store.AddHistory(uid, "gen", "alice 的图", "m", "1024x1024", "high", 1, []string{"a.png"})
	store.AddHistory(u.ID, "gen", "root 的图", "m", "1024x1024", "high", 1, []string{"b.png"})
	if h := store.ListHistory(uid, 100); len(h) != 1 || h[0].Prompt != "alice 的图" {
		t.Fatalf("用户历史隔离失败: %+v", h)
	}
	if h := store.ListAllHistory(100); len(h) != 2 {
		t.Fatalf("管理员应看到全部历史,实际 %d", len(h))
	}

	// 图片配额裁剪:limit=2,塞 3 条单图历史后应裁回 2 张
	store.UpdateUserImageLimit(uid, 2)
	for _, f := range []string{"c.png", "d.png", "e.png"} {
		store.AddHistory(uid, "gen", "p", "m", "s", "q", 1, []string{f})
	}
	removed := store.PruneUserImages(uid, 2)
	if len(removed) == 0 {
		t.Fatal("应裁掉最旧图片")
	}
	left := 0
	for _, h := range store.ListHistory(uid, 100) {
		left += len(h.Files)
	}
	if left > 2 {
		t.Fatalf("裁剪后应 <=2 张,实际 %d", left)
	}
}

func TestCaptcha(t *testing.T) {
	id, img := NewCaptcha()
	if len(img) < 100 {
		t.Fatal("验证码图片为空")
	}
	capMu.Lock()
	ans := capStore[id].answer
	capMu.Unlock()
	if len(ans) != captchaLen {
		t.Fatalf("答案长度异常: %q", ans)
	}
	if VerifyCaptcha("unknown-id", ans) {
		t.Fatal("未知 id 竟通过")
	}
	if !VerifyCaptcha(id, ans) {
		t.Fatal("正确验证码校验失败")
	}
	if VerifyCaptcha(id, ans) {
		t.Fatal("验证码应当一次性")
	}
}
