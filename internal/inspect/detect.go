package inspect

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

// =============================================================================
// 数值小工具
// =============================================================================

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func largestPow2LE(n int) int {
	if n < 1 {
		return 1
	}
	p := 1
	for p*2 <= n {
		p <<= 1
	}
	return p
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func ratio(w, h int) float64 {
	if h == 0 {
		return 0
	}
	return float64(w) / float64(h)
}

func meanSlice(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var s float64
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func maxSlice(x []float64) float64 {
	m := math.Inf(-1)
	for _, v := range x {
		if v > m {
			m = v
		}
	}
	return m
}
func maxFloat(x []float64) float64 { return maxSlice(x) }
func minFloat(x []float64) float64 {
	m := math.Inf(1)
	for _, v := range x {
		if v < m {
			m = v
		}
	}
	return m
}

func median(x []float64) float64 {
	n := len(x)
	if n == 0 {
		return 0
	}
	s := append([]float64(nil), x...)
	sort.Float64s(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return 0.5 * (s[n/2-1] + s[n/2])
}

// movavg5 复刻 np.convolve(x, ones(5)/5, "same"):零填充边界的居中 5 点平均。
// 与 Python moving_average 一致:len(x)<=5 时原样返回。
func movavg5(x []float64) []float64 {
	n := len(x)
	if n <= 5 {
		return x
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		var s float64
		for k := i - 2; k <= i+2; k++ {
			if k >= 0 && k < n {
				s += x[k]
			}
		}
		out[i] = s / 5.0
	}
	return out
}

// hann 复刻 numpy.hanning(n)。
func hann(n int) []float64 {
	w := make([]float64, n)
	if n == 1 {
		w[0] = 1
		return w
	}
	for i := 0; i < n; i++ {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	}
	return w
}

func meanStd(g [][]float64) (mean, std float64) {
	var s, n float64
	for _, row := range g {
		for _, v := range row {
			s += v
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	mean = s / n
	var ss float64
	for _, row := range g {
		for _, v := range row {
			d := v - mean
			ss += d * d
		}
	}
	std = math.Sqrt(ss / n)
	return
}

func localMaxMin(a [][]float64, i, j int) (lmax, lmin float64) {
	lmax = math.Inf(-1)
	lmin = math.Inf(1)
	for di := -1; di <= 1; di++ {
		for dj := -1; dj <= 1; dj++ {
			v := a[i+di][j+dj]
			if v > lmax {
				lmax = v
			}
			if v < lmin {
				lmin = v
			}
		}
	}
	return
}

func roundFactor(f float64) float64 {
	for _, cand := range []float64{1.25, 1.5, 2.0, 2.5, 3.0, 4.0, 6.0, 8.0} {
		if math.Abs(f-cand) <= 0.18 {
			return cand
		}
	}
	return math.Round(f*100) / 100
}

// pyFloat 复刻 Python str(float):整数值给 "N.0",否则最短表示("1.79")。
func pyFloat(f float64) string {
	if f == math.Trunc(f) {
		return strconv.FormatFloat(f, 'f', 1, 64)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func prepend(s []string, v string) []string {
	return append([]string{v}, s...)
}

func sortedUnique(s []string) []string {
	m := map[string]struct{}{}
	for _, v := range s {
		m[v] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// =============================================================================
// 平面提取 / 裁剪(返回行切片视图,只读用,省内存)
// =============================================================================

// fullLuminance 提取整幅亮度(0.299R+0.587G+0.114B)。
func fullLuminance(im *rgbImg) [][]float64 {
	out := make([][]float64, im.h)
	for y := 0; y < im.h; y++ {
		row := make([]float64, im.w)
		for x := 0; x < im.w; x++ {
			r, g, b := im.rgbAt(x, y)
			row[x] = 0.299*r + 0.587*g + 0.114*b
		}
		out[y] = row
	}
	return out
}

// centerCrop 复刻 Python center_crop:max(h,w)<=maxside 原样返回,否则中心裁剪到 ≤maxside。
func centerCrop(a [][]float64, maxside int) [][]float64 {
	h := len(a)
	if h == 0 {
		return a
	}
	w := len(a[0])
	if maxInt(h, w) <= maxside {
		return a
	}
	nh, nw := minInt(h, maxside), minInt(w, maxside)
	y0, x0 := (h-nh)/2, (w-nw)/2
	out := make([][]float64, nh)
	for i := 0; i < nh; i++ {
		out[i] = a[y0+i][x0 : x0+nw]
	}
	return out
}

// centerCropPow2 中心裁剪到 ≤cap 的「2 的幂」边长(供 2D FFT 快路径)。
func centerCropPow2(a [][]float64, cap int) [][]float64 {
	h := len(a)
	if h == 0 {
		return a
	}
	w := len(a[0])
	nh, nw := largestPow2LE(minInt(h, cap)), largestPow2LE(minInt(w, cap))
	if nh < 1 || nw < 1 {
		return a
	}
	y0, x0 := (h-nh)/2, (w-nw)/2
	out := make([][]float64, nh)
	for i := 0; i < nh; i++ {
		out[i] = a[y0+i][x0 : x0+nw]
	}
	return out
}

// planePow2 直接从图片中心裁剪 ≤cap 的 2 的幂区域,并用 sel 提取通道/色彩空间值。
func planePow2(im *rgbImg, cap int, sel func(r, g, b float64) float64) [][]float64 {
	nh := largestPow2LE(minInt(im.h, cap))
	nw := largestPow2LE(minInt(im.w, cap))
	y0, x0 := (im.h-nh)/2, (im.w-nw)/2
	out := make([][]float64, nh)
	for i := 0; i < nh; i++ {
		row := make([]float64, nw)
		for j := 0; j < nw; j++ {
			r, g, b := im.rgbAt(x0+j, y0+i)
			row[j] = sel(r, g, b)
		}
		out[i] = row
	}
	return out
}

// planeCenter 中心裁剪 ≤maxside(非 2 的幂),用于噪声/色差等非 FFT 计算。
func planeCenter(im *rgbImg, maxside int, sel func(r, g, b float64) float64) [][]float64 {
	nh, nw := minInt(im.h, maxside), minInt(im.w, maxside)
	y0, x0 := (im.h-nh)/2, (im.w-nw)/2
	out := make([][]float64, nh)
	for i := 0; i < nh; i++ {
		row := make([]float64, nw)
		for j := 0; j < nw; j++ {
			r, g, b := im.rgbAt(x0+j, y0+i)
			row[j] = sel(r, g, b)
		}
		out[i] = row
	}
	return out
}

// =============================================================================
// 频域:有效分辨率
// =============================================================================

type effRes struct {
	cutoff, upscale, upscaleRounded, hfRatio, plateau, tailDropDB float64
	centers, whitened                                             []float64
}

// radialPowerSpectrum 计算白化前的径向功率谱(nbins 个频带),频率归一到 [0,1](1=奈奎斯特)。
func radialPowerSpectrum(plane [][]float64, nbins int) (centers, prof []float64) {
	cropped := centerCropPow2(plane, fftCap)
	h := len(cropped)
	w := len(cropped[0])
	wy := hann(h)
	wx := hann(w)
	var mean float64
	for i := 0; i < h; i++ {
		for j := 0; j < w; j++ {
			mean += cropped[i][j]
		}
	}
	mean /= float64(h * w)
	re := make([][]float64, h)
	for i := 0; i < h; i++ {
		row := make([]float64, w)
		for j := 0; j < w; j++ {
			row[j] = (cropped[i][j] - mean) * wy[i] * wx[j]
		}
		re[i] = row
	}
	power := fft2Power(re)
	hw := float64(w) / 2.0
	hh := float64(h) / 2.0
	sums := make([]float64, nbins)
	counts := make([]int, nbins)
	for i := 0; i < h; i++ {
		fy := i
		if 2*i >= h {
			fy = i - h
		}
		ny := float64(fy) / hh
		for j := 0; j < w; j++ {
			fx := j
			if 2*j >= w {
				fx = j - w
			}
			nx := float64(fx) / hw
			r := math.Sqrt(nx*nx + ny*ny)
			if r > 1.0 {
				continue
			}
			idx := int(r * float64(nbins))
			if idx < 0 {
				idx = 0
			}
			if idx >= nbins {
				idx = nbins - 1
			}
			sums[idx] += power[i][j]
			counts[idx]++
		}
	}
	centers = make([]float64, nbins)
	prof = make([]float64, nbins)
	for k := 0; k < nbins; k++ {
		lo := float64(k) / float64(nbins)
		hi := float64(k+1) / float64(nbins)
		centers[k] = 0.5 * (lo + hi)
		c := counts[k]
		if c < 1 {
			c = 1
		}
		prof[k] = sums[k] / float64(c)
	}
	return
}

func estimateEffectiveResolution(plane [][]float64, cfg Config) *effRes {
	centersAll, profAll := radialPowerSpectrum(plane, 160)
	var c, p []float64
	for i := range centersAll {
		if centersAll[i] > 0.04 {
			c = append(c, centersAll[i])
			p = append(p, profAll[i])
		}
	}
	if len(p) == 0 || maxSlice(p) <= 0 {
		return nil
	}
	pc := make([]float64, len(c))
	for i := range c {
		pc[i] = p[i] * c[i] * c[i]
	}
	W := movavg5(pc)
	Wmax := maxSlice(W)
	Wn := make([]float64, len(W))
	for i := range W {
		Wn[i] = W[i] / (Wmax + 1e-12)
	}
	var pl []float64
	for i := range c {
		if c[i] >= 0.06 && c[i] <= 0.35 {
			pl = append(pl, Wn[i])
		}
	}
	plateau := maxSlice(Wn)
	if len(pl) > 0 {
		plateau = median(pl)
	}
	if plateau <= 0 {
		return nil
	}
	thr := plateau * math.Pow(10, cfg.WhitenDropDB/10.0)
	cutoff := c[0]
	found := false
	for i := range c {
		if Wn[i] > thr {
			if !found || c[i] > cutoff {
				cutoff = c[i]
				found = true
			}
		}
	}
	var hf []float64
	for i := range c {
		if c[i] >= 0.80 && c[i] <= 0.95 {
			hf = append(hf, Wn[i])
		}
	}
	hfLevel := 0.0
	if len(hf) > 0 {
		hfLevel = median(hf)
	}
	hfRatio := hfLevel / (plateau + 1e-12)
	tailThresh := math.Min(0.98, cutoff+0.05)
	var tl []float64
	for i := range c {
		if c[i] > tailThresh {
			tl = append(tl, Wn[i])
		}
	}
	tail := hfLevel
	if len(tl) > 0 {
		tail = median(tl)
	}
	dropDB := 10 * math.Log10((plateau+1e-12)/(tail+1e-12))
	upscale := 1.0
	if cutoff > 0 {
		upscale = 1.0 / cutoff
	}
	return &effRes{cutoff, upscale, roundFactor(upscale), hfRatio, plateau, dropDB, c, Wn}
}

// =============================================================================
// 重采样 / 插值指纹
// =============================================================================

type peak struct {
	freq, period, prominence float64
	axis                     string
}

type interpItem struct {
	kind, text         string
	period, prominence float64
}

type resampleOut struct {
	peaks          []peak
	interpretation []interpItem
}

func peaksFromProfile(prof []float64, cfg Config, axis string) []peak {
	n := len(prof)
	m := meanSlice(prof)
	win := hann(n)
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = (prof[i] - m) * win[i]
	}
	spec := rfftMag(x)
	half := len(spec)
	freqs := make([]float64, half)
	for i := 0; i < half; i++ {
		freqs[i] = float64(i) / float64(n)
	}
	const fmin = 0.02
	var medVals []float64
	for i := 0; i < half; i++ {
		if freqs[i] > fmin {
			medVals = append(medVals, spec[i])
		}
	}
	med := median(medVals) + 1e-12
	var peaks []peak
	for i := 1; i < half-1; i++ {
		if freqs[i] < fmin {
			continue
		}
		if spec[i] > spec[i-1] && spec[i] >= spec[i+1] {
			prom := spec[i] / med
			if prom >= cfg.PeakProminence {
				period := 0.0
				if freqs[i] > 0 {
					period = 1.0 / freqs[i]
				}
				peaks = append(peaks, peak{freqs[i], period, prom, axis})
			}
		}
	}
	sort.SliceStable(peaks, func(a, b int) bool { return peaks[a].prominence > peaks[b].prominence })
	if len(peaks) > 6 {
		peaks = peaks[:6]
	}
	return peaks
}

func detectResampling(lum [][]float64, cfg Config) resampleOut {
	h := len(lum)
	w := len(lum[0])
	var all []peak
	if h-2 >= 16 {
		prof := make([]float64, h-2)
		for i := 0; i < h-2; i++ {
			var s float64
			for j := 0; j < w; j++ {
				s += math.Abs(lum[i+2][j] - 2*lum[i+1][j] + lum[i][j])
			}
			prof[i] = s / float64(w)
		}
		all = append(all, peaksFromProfile(prof, cfg, "纵向")...)
	}
	if w-2 >= 16 {
		prof := make([]float64, w-2)
		for j := 0; j < w-2; j++ {
			var s float64
			for i := 0; i < h; i++ {
				s += math.Abs(lum[i][j+2] - 2*lum[i][j+1] + lum[i][j])
			}
			prof[j] = s / float64(h)
		}
		all = append(all, peaksFromProfile(prof, cfg, "横向")...)
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].prominence > all[b].prominence })
	out := resampleOut{}
	if len(all) > 6 {
		out.peaks = all[:6]
	} else {
		out.peaks = all
	}
	seen := map[int]bool{}
	for _, pk := range all {
		key := int(math.Round(pk.period))
		if seen[key] {
			continue
		}
		seen[key] = true
		p := pk.period
		switch {
		case p >= 7.4 && p <= 8.6:
			out.interpretation = append(out.interpretation, interpItem{"JPEG", "检测到周期≈8 像素的块栅格 -> JPEG 压缩痕迹", p, pk.prominence})
		case p >= 15 && p <= 17:
			out.interpretation = append(out.interpretation, interpItem{"UPSCALE_JPEG", "检测到周期≈16 像素栅格 -> 源 JPEG 被放大约 2 倍", p, pk.prominence})
		case p >= 23 && p <= 25:
			out.interpretation = append(out.interpretation, interpItem{"UPSCALE_JPEG", "检测到周期≈24 像素栅格 -> 源 JPEG 被放大约 3 倍", p, pk.prominence})
		case p >= 31 && p <= 33:
			out.interpretation = append(out.interpretation, interpItem{"UPSCALE_JPEG", "检测到周期≈32 像素栅格 -> 源 JPEG 被放大约 4 倍", p, pk.prominence})
		case p >= 1.7 && p <= 4.5:
			out.interpretation = append(out.interpretation, interpItem{"RESAMPLE",
				fmt.Sprintf("检测到周期≈%.1f 像素的重采样栅格 -> 约 %s 倍放大插值", p, pyFloat(roundFactor(p))), p, pk.prominence})
		}
	}
	return out
}

// =============================================================================
// 最近邻
// =============================================================================

func nnBlockConsistency(q [][]int, N int) float64 {
	h := len(q)
	w := len(q[0])
	hh := h - h%N
	ww := w - w%N
	if hh < N || ww < N {
		return 0
	}
	var eq, tot float64
	for bi := 0; bi < hh; bi += N {
		for bj := 0; bj < ww; bj += N {
			anchor := q[bi][bj]
			for di := 0; di < N; di++ {
				for dj := 0; dj < N; dj++ {
					tot++
					if q[bi+di][bj+dj] == anchor {
						eq++
					}
				}
			}
		}
	}
	return safeDiv(eq, tot)
}

func detectNearestNeighbor(lum [][]float64, cfg Config) Nearest {
	c := centerCrop(lum, 1400)
	h := len(c)
	w := len(c[0])
	q := make([][]int, h)
	for i := 0; i < h; i++ {
		row := make([]int, w)
		for j := 0; j < w; j++ {
			row[j] = int(math.Round(c[i][j]))
		}
		q[i] = row
	}
	var hEq, hTot, vEq, vTot float64
	for i := 0; i < h; i++ {
		for j := 1; j < w; j++ {
			hTot++
			if q[i][j] == q[i][j-1] {
				hEq++
			}
		}
	}
	for i := 1; i < h; i++ {
		for j := 0; j < w; j++ {
			vTot++
			if q[i][j] == q[i-1][j] {
				vEq++
			}
		}
	}
	eq := math.Max(safeDiv(hEq, hTot), safeDiv(vEq, vTot))
	var factor *float64
	bestC := 0.0
	if eq >= cfg.NNEqualFrac {
		for _, N := range []int{2, 3, 4, 6, 8} {
			cc := nnBlockConsistency(q, N)
			if cc >= 0.995 {
				f := float64(N)
				factor = &f
				bestC = cc
			}
		}
	}
	return Nearest{EqualFraction: eq, IsNN: factor != nil, Factor: factor, BlockConsistency: bestC}
}

// =============================================================================
// 噪声 / 锐化
// =============================================================================

func estimateNoise(a [][]float64) *float64 {
	h := len(a)
	if h < 3 {
		return nil
	}
	w := len(a[0])
	if w < 3 {
		return nil
	}
	var s float64
	for i := 1; i < h-1; i++ {
		for j := 1; j < w-1; j++ {
			v := a[i-1][j-1] - 2*a[i-1][j] + a[i-1][j+1] -
				2*a[i][j-1] + 4*a[i][j] - 2*a[i][j+1] +
				a[i+1][j-1] - 2*a[i+1][j] + a[i+1][j+1]
			s += math.Abs(v)
		}
	}
	val := s * math.Sqrt(0.5*math.Pi) / (6.0 * float64(w-2) * float64(h-2))
	return &val
}

func detectSharpening(a [][]float64, cfg Config) Sharpen {
	h := len(a)
	if h < 5 {
		return Sharpen{}
	}
	w := len(a[0])
	if w < 5 {
		return Sharpen{}
	}
	g := make([][]float64, h)
	for i := 0; i < h; i++ {
		g[i] = make([]float64, w)
	}
	for i := 0; i < h; i++ {
		for j := 0; j < w-1; j++ {
			g[i][j] += math.Abs(a[i][j+1] - a[i][j])
		}
	}
	for i := 0; i < h-1; i++ {
		for j := 0; j < w; j++ {
			g[i][j] += math.Abs(a[i+1][j] - a[i][j])
		}
	}
	mean, std := meanStd(g)
	thr := mean + 2*std
	var overStrong, strongCount float64
	for i := 1; i < h-1; i++ {
		for j := 1; j < w-1; j++ {
			lmax, lmin := localMaxMin(a, i, j)
			center := a[i][j]
			rng := lmax - lmin + 1e-6
			overshoot := center >= lmax-0.02*rng || center <= lmin+0.02*rng
			strongC := g[i][j] > thr && rng > 25
			if strongC {
				strongCount++
				if overshoot {
					overStrong++
				}
			}
		}
	}
	var r float64
	if strongCount >= 30 {
		r = overStrong / strongCount
	}
	return Sharpen{OvershootRatio: r, Sharpened: r > cfg.OvershootThresh}
}

// =============================================================================
// RGB 三通道 / 色度 / 色差
// =============================================================================

func perChannelAnalysis(im *rgbImg, cfg Config, longSide int) map[string]Channel {
	sels := map[string]func(r, g, b float64) float64{
		"R": func(r, _, _ float64) float64 { return r },
		"G": func(_, g, _ float64) float64 { return g },
		"B": func(_, _, b float64) float64 { return b },
	}
	out := map[string]Channel{}
	for _, ch := range []string{"R", "G", "B"} {
		eff := estimateEffectiveResolution(planePow2(im, fftCap, sels[ch]), cfg)
		var cutoff, effpx *float64
		if eff != nil {
			c := eff.cutoff
			cutoff = &c
			e := c * float64(longSide)
			effpx = &e
		}
		n := estimateNoise(planeCenter(im, 1600, sels[ch]))
		out[ch] = Channel{Cutoff: cutoff, EffPx: effpx, Noise: n}
	}
	return out
}

func chromaAnalysis(im *rgbImg, cfg Config) Chroma {
	yP := planePow2(im, fftCap, func(r, g, b float64) float64 { return 0.299*r + 0.587*g + 0.114*b })
	cbP := planePow2(im, fftCap, func(r, g, b float64) float64 { return -0.168736*r - 0.331264*g + 0.5*b + 128 })
	crP := planePow2(im, fftCap, func(r, g, b float64) float64 { return 0.5*r - 0.418688*g - 0.081312*b + 128 })
	effY := estimateEffectiveResolution(yP, cfg)
	effCb := estimateEffectiveResolution(cbP, cfg)
	effCr := estimateEffectiveResolution(crP, cfg)
	var cy, cc *float64
	if effY != nil {
		v := effY.cutoff
		cy = &v
	}
	if effCb != nil && effCr != nil {
		v := 0.5 * (effCb.cutoff + effCr.cutoff)
		cc = &v
	}
	var rt *float64
	if cy != nil && cc != nil && *cy > 0 {
		v := *cc / *cy
		rt = &v
	}
	sub := ""
	if rt != nil && cy != nil && *cy >= 0.7 {
		switch {
		case *rt < 0.6:
			sub = "色度分辨率约为亮度一半 -> 4:2:0 色度下采样(JPEG 常见)"
		case *rt < 0.85:
			sub = "色度略低于亮度 -> 轻度色度下采样(4:2:2 类)"
		default:
			sub = "色度与亮度分辨率接近 -> 4:4:4 / 无明显色度下采样"
		}
	}
	return Chroma{LumaCutoff: cy, ChromaCutoff: cc, Ratio: rt, Subsample: sub}
}

func highpass3(a [][]float64) [][]float64 {
	h := len(a)
	w := len(a[0])
	out := make([][]float64, h-2)
	for i := 1; i < h-1; i++ {
		row := make([]float64, w-2)
		for j := 1; j < w-1; j++ {
			var s float64
			for di := -1; di <= 1; di++ {
				for dj := -1; dj <= 1; dj++ {
					s += a[i+di][j+dj]
				}
			}
			row[j-1] = a[i][j] - s/9.0
		}
		out[i-1] = row
	}
	return out
}

func subMean(a [][]float64) {
	var s, n float64
	for _, row := range a {
		for _, v := range row {
			s += v
			n++
		}
	}
	if n == 0 {
		return
	}
	m := s / n
	for i := range a {
		for j := range a[i] {
			a[i][j] -= m
		}
	}
}

func chromaticAberration(im *rgbImg) CA {
	r := planeCenter(im, 700, func(r, _, _ float64) float64 { return r })
	b := planeCenter(im, 700, func(_, _, b float64) float64 { return b })
	if len(r) < 5 || len(r[0]) < 5 {
		return CA{Present: false}
	}
	hr := highpass3(r)
	hb := highpass3(b)
	subMean(hr)
	subMean(hb)
	H := len(hr)
	Wd := len(hr[0])
	if H < 5 || Wd < 5 {
		return CA{Present: false}
	}
	var base float64
	for i := 0; i < H; i++ {
		for j := 0; j < Wd; j++ {
			base += hr[i][j] * hb[i][j]
		}
	}
	bestVal := base
	bx, by := 0, 0
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			var val float64
			for i := 2; i < H-2; i++ {
				for j := 2; j < Wd-2; j++ {
					val += hr[i][j] * hb[i+dy][j+dx]
				}
			}
			if val > bestVal {
				bestVal = val
				bx, by = dx, dy
			}
		}
	}
	mag := math.Hypot(float64(bx), float64(by))
	return CA{ShiftX: bx, ShiftY: by, Magnitude: mag, Present: mag >= 1.0}
}

// =============================================================================
// 内容类型自动识别
// =============================================================================

func guessContentType(im *rgbImg) string {
	nh, nw := minInt(im.h, 800), minInt(im.w, 800)
	y0, x0 := (im.h-nh)/2, (im.w-nw)/2
	lum := make([][]float64, nh)
	for i := 0; i < nh; i++ {
		row := make([]float64, nw)
		for j := 0; j < nw; j++ {
			r, g, b := im.rgbAt(x0+j, y0+i)
			row[j] = 0.299*r + 0.587*g + 0.114*b
		}
		lum[i] = row
	}
	noise := 0.0
	if n := estimateNoise(lum); n != nil {
		noise = *n
	}
	var flat, ftot float64
	for i := 1; i < nh-1; i++ {
		for j := 1; j < nw-1; j++ {
			lmax, lmin := localMaxMin(lum, i, j)
			ftot++
			if lmax-lmin < 3 {
				flat++
			}
		}
	}
	flatRatio := safeDiv(flat, ftot)
	uniq := map[int]struct{}{}
	var satSum, n float64
	for i := 0; i < nh; i++ {
		for j := 0; j < nw; j++ {
			r, g, b := im.rgbAt(x0+j, y0+i)
			code := (int(r)/16)*256 + (int(g)/16)*16 + (int(b) / 16)
			uniq[code] = struct{}{}
			mx := math.Max(r, math.Max(g, b))
			mn := math.Min(r, math.Min(g, b))
			satSum += (mx - mn) / (mx + 1e-6)
			n++
		}
	}
	sat := safeDiv(satSum, n)
	uniqN := len(uniq)
	lowNoise := noise < 1.6
	if flatRatio > 0.5 && lowNoise {
		if sat > 0.2 && uniqN > 150 {
			return "anime"
		}
		return "screenshot"
	}
	if lowNoise && sat > 0.35 && uniqN > 150 && flatRatio > 0.12 {
		return "anime"
	}
	if lowNoise && flatRatio > 0.3 {
		return "game"
	}
	return "photo"
}
