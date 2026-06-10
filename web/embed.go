// Package web 把构建好的前端 dist 目录嵌入二进制。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist 返回 dist 子目录作为只读文件系统(供 http.FileServer 使用)。
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
