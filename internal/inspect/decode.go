package inspect

import (
	"bytes"
	"image"
	"image/draw"
	"strings"

	// 注册标准库解码器
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	// 注册 golang.org/x/image 解码器(纯 Go,无 cgo)
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// rgbImg 是去预乘的 8bit RGBA 缓冲(行距固定 4*w),rgbAt 取 R/G/B(忽略 alpha,
// 对齐 PIL 的 convert("RGB"):直接丢弃 alpha 通道)。
type rgbImg struct {
	w, h int
	pix  []uint8
}

func (im *rgbImg) rgbAt(x, y int) (r, g, b float64) {
	o := (y*im.w + x) * 4
	return float64(im.pix[o]), float64(im.pix[o+1]), float64(im.pix[o+2])
}

func decode(data []byte) (*rgbImg, string, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return &rgbImg{w: b.Dx(), h: b.Dy(), pix: dst.Pix}, normFormat(format), nil
}

func normFormat(f string) string {
	switch strings.ToLower(f) {
	case "jpeg", "jpg":
		return "JPEG"
	case "png":
		return "PNG"
	case "gif":
		return "GIF"
	case "webp":
		return "WEBP"
	case "bmp":
		return "BMP"
	case "tiff":
		return "TIFF"
	}
	return strings.ToUpper(f)
}
