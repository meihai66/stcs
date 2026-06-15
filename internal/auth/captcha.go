package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"math/big"
	"sync"
	"time"
)

// 图形验证码:服务端生成 4 位随机数字的扭曲图片,答案存内存(带 TTL,一次性)。
// 用自带的 5x7 位图字体绘制,无任何外部字体依赖。

const (
	captchaLen = 4
	captchaTTL = 3 * time.Minute
)

// digitFont 是 0-9 的 5x7 位图,每行低 5 位(bit4 为最左列)。
var digitFont = [10][7]byte{
	{14, 17, 19, 21, 25, 17, 14}, // 0
	{4, 12, 4, 4, 4, 4, 14},      // 1
	{14, 17, 1, 2, 4, 8, 31},     // 2
	{31, 2, 4, 2, 1, 17, 14},     // 3
	{2, 6, 10, 18, 31, 2, 2},     // 4
	{31, 16, 30, 1, 1, 17, 14},   // 5
	{6, 8, 16, 30, 17, 17, 14},   // 6
	{31, 1, 2, 4, 8, 8, 8},       // 7
	{14, 17, 17, 14, 17, 17, 14}, // 8
	{14, 17, 17, 15, 1, 2, 12},   // 9
}

type captchaItem struct {
	answer  string
	expires time.Time
}

var (
	capMu    sync.Mutex
	capStore = map[string]captchaItem{}
)

// randInt 返回 [0,n) 的随机数(crypto/rand)。
func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// NewCaptcha 生成一个验证码,返回 id 与 PNG 的 base64(data URI)。
func NewCaptcha() (id, dataURI string) {
	digits := make([]byte, captchaLen)
	for i := range digits {
		digits[i] = byte('0' + randInt(10))
	}
	answer := string(digits)
	idb := make([]byte, 12)
	_, _ = rand.Read(idb)
	id = hex.EncodeToString(idb)

	capMu.Lock()
	// 顺手清理过期项
	now := time.Now()
	for k, v := range capStore {
		if now.After(v.expires) {
			delete(capStore, k)
		}
	}
	capStore[id] = captchaItem{answer: answer, expires: now.Add(captchaTTL)}
	capMu.Unlock()

	return id, "data:image/png;base64," + base64.StdEncoding.EncodeToString(renderCaptcha(answer))
}

// VerifyCaptcha 校验并消费一个验证码(一次性)。
func VerifyCaptcha(id, answer string) bool {
	capMu.Lock()
	defer capMu.Unlock()
	item, ok := capStore[id]
	if !ok {
		return false
	}
	delete(capStore, id) // 一次性,无论对错都消费
	if time.Now().After(item.expires) {
		return false
	}
	return item.answer == answer
}

// renderCaptcha 把数字串画成带干扰的 PNG。
func renderCaptcha(text string) []byte {
	const scale = 6
	cellW := 5*scale + 10 // 每字符占位宽(含间距)
	w := len(text)*cellW + 12
	h := 7*scale + 20

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{R: 244, G: 245, B: 250, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, bg)
		}
	}

	// 背景噪点
	for i := 0; i < w*h/14; i++ {
		g := uint8(200 + randInt(40))
		img.Set(randInt(w), randInt(h), color.RGBA{R: g, G: g, B: g, A: 255})
	}

	// 字符
	for i := 0; i < len(text); i++ {
		glyph := digitFont[text[i]-'0']
		col := color.RGBA{R: uint8(randInt(110)), G: uint8(randInt(110)), B: uint8(randInt(150)), A: 255}
		ox := 8 + i*cellW + randInt(4)
		oy := 7 + randInt(6)
		for r := 0; r < 7; r++ {
			for c := 0; c < 5; c++ {
				if glyph[r]&(1<<(4-c)) != 0 {
					fillRect(img, ox+c*scale, oy+r*scale, scale, scale, col)
				}
			}
		}
	}

	// 几条干扰线
	for i := 0; i < 4; i++ {
		col := color.RGBA{R: uint8(randInt(180)), G: uint8(randInt(180)), B: uint8(randInt(200)), A: 255}
		drawLine(img, randInt(w), randInt(h), randInt(w), randInt(h), col)
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func fillRect(img *image.RGBA, x, y, w, h int, col color.RGBA) {
	b := img.Bounds()
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			px, py := x+dx, y+dy
			if px >= 0 && py >= 0 && px < b.Dx() && py < b.Dy() {
				img.Set(px, py, col)
			}
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, col color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		img.Set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
