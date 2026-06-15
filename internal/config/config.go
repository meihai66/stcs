// Package config 负责读写「每个用户」的中转站配置(多套 profile,可切换)。
//
// 配置以 JSON 存在 users.config 列(经 store 读写),结构:
//
//	{ "profiles":[{name,base_url,api_key,model,reverse_model,request_format}],
//	  "active_profile","default_size","default_quality","timeout" }
//
// 全局项(worker 并发数、对外 API 的 server_api_key)放在 settings 表,管理员统一设。
package config

import (
	"encoding/json"
	"os"
	"strconv"
	"sync"

	"github.com/meihai66/stcs/internal/store"
)

// Profile 是一套中转站独有的配置。
type Profile struct {
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key"`
	Model         string `json:"model"`
	ReverseModel  string `json:"reverse_model"`
	RequestFormat string `json:"request_format"`
}

// raw 是某用户配置 JSON 的完整结构。
type raw struct {
	Profiles       []Profile `json:"profiles"`
	ActiveProfile  string    `json:"active_profile"`
	DefaultSize    string    `json:"default_size"`
	DefaultQuality string    `json:"default_quality"`
	Timeout        int       `json:"timeout"`
	// 兼容旧全局 config.json 迁移而来的字段(读时容忍,不再写出)
	Concurrency  int    `json:"concurrency,omitempty"`
	ServerAPIKey string `json:"server_api_key,omitempty"`
}

// Flat 是扁平化后的配置,业务层只认这个。
type Flat struct {
	BaseURL        string
	APIKey         string
	Model          string
	ReverseModel   string
	RequestFormat  string
	DefaultSize    string
	DefaultQuality string
	Timeout        int
	Concurrency    int
	ServerAPIKey   string
	ActiveProfile  string
}

var profileDefaults = Profile{
	BaseURL: "", APIKey: "", Model: "gpt-image-2",
	ReverseModel: "gpt-4o", RequestFormat: "images",
}

var mu sync.Mutex

func readRaw(userID int64) raw {
	var r raw
	if s := store.GetUserConfig(userID); s != "" {
		_ = json.Unmarshal([]byte(s), &r)
	}
	if r.Profiles == nil {
		r.Profiles = []Profile{}
	}
	if r.DefaultSize == "" {
		r.DefaultSize = "1024x1024"
	}
	if r.DefaultQuality == "" {
		r.DefaultQuality = "high"
	}
	if r.Timeout == 0 {
		r.Timeout = 300
	}
	if r.ActiveProfile == "" && len(r.Profiles) > 0 {
		r.ActiveProfile = r.Profiles[0].Name
	}
	return r
}

func writeRaw(userID int64, r raw) {
	r.Concurrency, r.ServerAPIKey = 0, "" // 旧字段不再写出
	b, _ := json.Marshal(r)
	store.SetUserConfig(userID, string(b))
}

func activeProfile(r raw) Profile {
	for _, p := range r.Profiles {
		if p.Name == r.ActiveProfile {
			return p
		}
	}
	if len(r.Profiles) > 0 {
		return r.Profiles[0]
	}
	return profileDefaults
}

func env(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// ----------------------------- 全局项 -----------------------------

// GlobalConcurrency 返回任务 worker 并发数(全局,管理员设;env 优先;默认 3)。
func GlobalConcurrency() int {
	if v := env("STCS_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	n, _ := strconv.Atoi(store.GetSetting("concurrency", ""))
	if n <= 0 {
		n = 3
	}
	return n
}

// GlobalServerAPIKey 返回对外 OpenAI 兼容 API 的密钥(全局;env 优先)。
func GlobalServerAPIKey() string {
	if v := env("STCS_SERVER_API_KEY", "GPTIMG_SERVER_API_KEY"); v != "" {
		return v
	}
	return store.GetSetting("server_api_key", "")
}

// SetGlobalConcurrency / SetGlobalServerAPIKey 由管理员设置入口调用。
func SetGlobalConcurrency(n int) {
	if n > 0 {
		store.SetSetting("concurrency", strconv.Itoa(n))
	}
}
func SetGlobalServerAPIKey(k string) { store.SetSetting("server_api_key", k) }

// ----------------------------- 每用户配置 -----------------------------

// LoadForUser 返回某用户的扁平配置(中转站来自其激活 profile;并发/对外密钥来自全局)。
func LoadForUser(userID int64) Flat {
	mu.Lock()
	defer mu.Unlock()
	r := readRaw(userID)
	p := activeProfile(r)
	f := Flat{
		BaseURL:        p.BaseURL,
		APIKey:         p.APIKey,
		Model:          orDefault(p.Model, profileDefaults.Model),
		ReverseModel:   orDefault(p.ReverseModel, profileDefaults.ReverseModel),
		RequestFormat:  orDefault(p.RequestFormat, profileDefaults.RequestFormat),
		DefaultSize:    r.DefaultSize,
		DefaultQuality: r.DefaultQuality,
		Timeout:        r.Timeout,
		ActiveProfile:  r.ActiveProfile,
		Concurrency:    GlobalConcurrency(),
		ServerAPIKey:   GlobalServerAPIKey(),
	}
	// 环境变量覆盖(部署用,作用于所有用户)
	if v := env("STCS_BASE_URL", "GPTIMG_BASE_URL"); v != "" {
		f.BaseURL = v
	}
	if v := env("STCS_API_KEY", "GPTIMG_API_KEY"); v != "" {
		f.APIKey = v
	}
	if v := env("STCS_MODEL", "GPTIMG_MODEL"); v != "" {
		f.Model = v
	}
	return f
}

// SaveGlobalForUser 保存某用户的非中转站全局项(尺寸/质量/超时)。
func SaveGlobalForUser(userID int64, updates map[string]any) Flat {
	mu.Lock()
	r := readRaw(userID)
	if v, ok := updates["default_quality"].(string); ok && v != "" {
		r.DefaultQuality = v
	}
	if v, ok := updates["default_size"].(string); ok && v != "" {
		r.DefaultSize = v
	}
	if v, ok := toInt(updates["timeout"]); ok && v > 0 {
		r.Timeout = v
	}
	writeRaw(userID, r)
	mu.Unlock()
	return LoadForUser(userID)
}

// ListProfilesForUser 返回某用户的所有 profile 与激活名。
func ListProfilesForUser(userID int64) ([]Profile, string) {
	mu.Lock()
	defer mu.Unlock()
	r := readRaw(userID)
	out := make([]Profile, len(r.Profiles))
	copy(out, r.Profiles)
	return out, r.ActiveProfile
}

// SaveProfileForUser 新增或按 name 更新某用户的一套配置。api_key 空或脱敏占位时不覆盖。
func SaveProfileForUser(userID int64, in Profile) error {
	if in.Name == "" {
		return ErrEmptyName
	}
	mu.Lock()
	defer mu.Unlock()
	r := readRaw(userID)
	var prof *Profile
	for i := range r.Profiles {
		if r.Profiles[i].Name == in.Name {
			prof = &r.Profiles[i]
			break
		}
	}
	if prof == nil {
		np := profileDefaults
		np.Name = in.Name
		r.Profiles = append(r.Profiles, np)
		prof = &r.Profiles[len(r.Profiles)-1]
	}
	if in.BaseURL != "" {
		prof.BaseURL = in.BaseURL
	}
	if in.APIKey != "" && !isMasked(in.APIKey) {
		prof.APIKey = in.APIKey
	}
	if in.Model != "" {
		prof.Model = in.Model
	}
	if in.ReverseModel != "" {
		prof.ReverseModel = in.ReverseModel
	}
	if in.RequestFormat != "" {
		prof.RequestFormat = in.RequestFormat
	}
	if r.ActiveProfile == "" {
		r.ActiveProfile = in.Name
	}
	writeRaw(userID, r)
	return nil
}

// DeleteProfileForUser 删除某用户的一套配置。
func DeleteProfileForUser(userID int64, name string) {
	mu.Lock()
	defer mu.Unlock()
	r := readRaw(userID)
	kept := r.Profiles[:0]
	for _, p := range r.Profiles {
		if p.Name != name {
			kept = append(kept, p)
		}
	}
	r.Profiles = kept
	if r.ActiveProfile == name {
		if len(r.Profiles) > 0 {
			r.ActiveProfile = r.Profiles[0].Name
		} else {
			r.ActiveProfile = ""
		}
	}
	writeRaw(userID, r)
}

// SetActiveForUser 切换某用户的激活配置。
func SetActiveForUser(userID int64, name string) {
	mu.Lock()
	defer mu.Unlock()
	r := readRaw(userID)
	for _, p := range r.Profiles {
		if p.Name == name {
			r.ActiveProfile = name
			writeRaw(userID, r)
			return
		}
	}
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func isMasked(s string) bool {
	return len(s) >= 4 && s[:4] == "****"
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case json.Number:
		i, err := x.Int64()
		return int(i), err == nil
	}
	return 0, false
}

// ErrEmptyName 在 profile 名为空时返回。
var ErrEmptyName = errStr("配置名不能为空")

type errStr string

func (e errStr) Error() string { return string(e) }
