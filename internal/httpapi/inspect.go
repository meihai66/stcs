package httpapi

import (
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/meihai66/stcs/internal/inspect"
)

// 单次上传的总大小与单文件大小上限(图片检测,纯内存解码,避免大文件打爆内存)。
const (
	inspectMaxTotal   = 128 << 20 // 128MB 整批
	inspectMaxPerFile = 40 << 20  // 40MB 单图
)

var inspectExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
	".bmp": true, ".tif": true, ".tiff": true, ".gif": true,
}

type inspectItem struct {
	Name   string          `json:"name"`
	OK     bool            `json:"ok"`
	Error  string          `json:"error,omitempty"`
	Result *inspect.Result `json:"result,omitempty"`
}

// handleInspect 接收 multipart 多图上传,逐张做画质/超分检测,返回结果数组。
// preset: auto|photo|anime|game|screenshot;claim: ""|8k|4k|2k|1k|720p。
func handleInspect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(inspectMaxTotal); err != nil {
		writeErr(w, http.StatusBadRequest, "解析上传失败:"+err.Error())
		return
	}
	preset := strings.TrimSpace(r.FormValue("preset"))
	if preset == "" {
		preset = "auto"
	}
	claim := strings.TrimSpace(r.FormValue("claim"))

	var files []*multipart.FileHeader
	if r.MultipartForm != nil {
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 {
		writeErr(w, http.StatusBadRequest, "请至少上传一张图片")
		return
	}
	if len(files) > 30 {
		files = files[:30]
	}

	results := make([]inspectItem, 0, len(files))
	for _, fh := range files {
		name := fh.Filename
		if name == "" {
			name = "未命名"
		}
		ext := strings.ToLower(filepath.Ext(name))
		if !inspectExt[ext] {
			results = append(results, inspectItem{Name: name, OK: false, Error: "不支持的文件类型(仅图片)"})
			continue
		}
		if fh.Size > inspectMaxPerFile {
			results = append(results, inspectItem{Name: name, OK: false, Error: "图片过大(超过 40MB)"})
			continue
		}
		data, err := readUpload(fh)
		if err != nil || len(data) == 0 {
			results = append(results, inspectItem{Name: name, OK: false, Error: "读取失败"})
			continue
		}
		res, err := inspect.Analyze(data, name, preset, claim)
		if err != nil {
			results = append(results, inspectItem{Name: name, OK: false, Error: "分析失败:" + err.Error()})
			continue
		}
		results = append(results, inspectItem{Name: name, OK: true, Result: res})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func readUpload(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, inspectMaxPerFile))
}
