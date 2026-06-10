// Package config 负责读写配置:支持多套「中转站 profile」,可保存多个、随时切换。
//
// config.json 结构与原 Python 版保持兼容:
//
//	{
//	  "profiles": [ {"name","base_url","api_key","model","reverse_model","request_format"}, ... ],
//	  "active_profile": "...",
//	  "default_size","default_quality","timeout","concurrency","server_api_key"
//	}
//
// Load() 返回「扁平」配置(全局项 + 当前激活 profile 的字段),供 generator/tasks/stress 直接使用。
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
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

// raw 是磁盘上 config.json 的完整结构。
type raw struct {
	Profiles      []Profile `json:"profiles"`
	ActiveProfile string    `json:"active_profile"`
	DefaultSize   string    `json:"default_size"`
	DefaultQuality string   `json:"default_quality"`
	Timeout       int       `json:"timeout"`
	Concurrency   int       `json:"concurrency"`
	ServerAPIKey  string    `json:"server_api_key"`
}

// Flat 是扁平化后的配置(全局项 + 当前 profile 字段),业务层只认这个。
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

var (
	mu   sync.Mutex
	path string
)

// Init 设定 config.json 路径。须在使用前调用一次。
func Init(configPath string) {
	path = configPath
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
}

func readRaw() raw {
	var r raw
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &r)
	}
	if r.Profiles == nil {
		r.Profiles = []Profile{}
	}
	// 全局默认
	if r.DefaultSize == "" {
		r.DefaultSize = "1024x1024"
	}
	if r.DefaultQuality == "" {
		r.DefaultQuality = "high"
	}
	if r.Timeout == 0 {
		r.Timeout = 300
	}
	if r.Concurrency == 0 {
		r.Concurrency = 3
	}
	if r.ActiveProfile == "" && len(r.Profiles) > 0 {
		r.ActiveProfile = r.Profiles[0].Name
	}
	return r
}

func writeRaw(r raw) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
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

// Load 返回扁平配置;环境变量优先(部署用)。
func Load() Flat {
	mu.Lock()
	defer mu.Unlock()
	r := readRaw()
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
		Concurrency:    r.Concurrency,
		ServerAPIKey:   r.ServerAPIKey,
		ActiveProfile:  r.ActiveProfile,
	}
	// 环境变量覆盖(同时兼容旧 GPTIMG_ 前缀)
	if v := env("STCS_BASE_URL", "GPTIMG_BASE_URL"); v != "" {
		f.BaseURL = v
	}
	if v := env("STCS_API_KEY", "GPTIMG_API_KEY"); v != "" {
		f.APIKey = v
	}
	if v := env("STCS_MODEL", "GPTIMG_MODEL"); v != "" {
		f.Model = v
	}
	if v := env("STCS_SERVER_API_KEY", "GPTIMG_SERVER_API_KEY"); v != "" {
		f.ServerAPIKey = v
	}
	return f
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// SaveGlobal 保存全局项;中转站字段(若给出)写入当前激活 profile(无则建「默认」)。
func SaveGlobal(updates map[string]any) Flat {
	mu.Lock()
	r := readRaw()
	if v, ok := updates["default_quality"].(string); ok && v != "" {
		r.DefaultQuality = v
	}
	if v, ok := toInt(updates["timeout"]); ok && v > 0 {
		r.Timeout = v
	}
	if v, ok := toInt(updates["concurrency"]); ok && v > 0 {
		r.Concurrency = v
	}
	if v, ok := updates["server_api_key"].(string); ok && v != "" && !isMasked(v) {
		r.ServerAPIKey = v
	}
	if v, ok := updates["default_size"].(string); ok && v != "" {
		r.DefaultSize = v
	}
	_ = writeRaw(r)
	mu.Unlock()
	return Load()
}

// ListProfiles 返回所有 profile 与当前激活名。
func ListProfiles() ([]Profile, string) {
	mu.Lock()
	defer mu.Unlock()
	r := readRaw()
	out := make([]Profile, len(r.Profiles))
	copy(out, r.Profiles)
	return out, r.ActiveProfile
}

// SaveProfile 新增或按 name 更新一套配置。api_key 为空或脱敏占位符时不覆盖原值。
func SaveProfile(in Profile) error {
	if in.Name == "" {
		return ErrEmptyName
	}
	mu.Lock()
	defer mu.Unlock()
	r := readRaw()
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
	return writeRaw(r)
}

// DeleteProfile 删除一套配置。
func DeleteProfile(name string) {
	mu.Lock()
	defer mu.Unlock()
	r := readRaw()
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
	_ = writeRaw(r)
}

// SetActive 切换当前激活配置。
func SetActive(name string) {
	mu.Lock()
	defer mu.Unlock()
	r := readRaw()
	for _, p := range r.Profiles {
		if p.Name == name {
			r.ActiveProfile = name
			_ = writeRaw(r)
			return
		}
	}
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
