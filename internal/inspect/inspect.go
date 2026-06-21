// Package inspect 是 image_inspector.py 的 Go 移植:对单张图片做画质/分辨率/超分(放大)检测。
// 仅用标准库 + golang.org/x/image 解码,保持零 cgo、单静态二进制。
// 检测项:分辨率分级、频域有效分辨率(FFT)、重采样指纹、最近邻、锐化、噪声、
// RGB 三通道、色度(YCbCr)下采样、镜头色差(CA)、元数据(JPEG 质量/软件痕迹),
// 综合给出 原生(OK)/处理过(WARN)/放大假(FAIL) 三档结论与原因。
package inspect

import (
	"fmt"
	"strings"
)

// fftCap:2D FFT 前把亮度/通道中心裁剪到 ≤ 此值的「2 的幂」边长,再走 radix-2。
// Python 用矩形裁剪到 1600;这里用方/矩形 2 的幂裁剪以走最快 FFT 路径,
// 频率截止是比例量,对裁剪尺寸不敏感,判定阈值留有余量,实测与 Python 判定一致。
const fftCap = 1024

// maxPixels:像素总数上限(约 5000 万,足够覆盖 8K)。超过则拒绝,避免单张超大图
// 解出几百 MB 浮点亮度面打爆本机内存(自托管小内存机)。
const maxPixels = 50_000_000

// =============================================================================
// 配置与预设
// =============================================================================

type Config struct {
	Name             string
	Desc             string
	CutoffNative     float64
	CutoffSuspect    float64
	WhitenDropDB     float64
	SharpenHFRatio   float64
	OvershootThresh  float64
	NoiseLow         float64
	NNEqualFrac      float64
	PeakProminence   float64
	FlagLowNoiseAsAI bool
	Note             string
}

func basePhoto() Config {
	return Config{
		Name: "photo", Desc: "通用照片(默认)",
		CutoffNative: 0.85, CutoffSuspect: 0.70, WhitenDropDB: -10.0,
		SharpenHFRatio: 1.60, OvershootThresh: 0.85, NoiseLow: 0.6,
		NNEqualFrac: 0.40, PeakProminence: 10.0, FlagLowNoiseAsAI: true,
		Note: "按真实相机照片调参:存在传感器噪点、连续色调。",
	}
}

// Presets 返回内容类型预设表(每次返回新副本,避免共享可变状态)。
func Presets() map[string]Config {
	photo := basePhoto()
	anime := basePhoto()
	anime.Name, anime.Desc = "anime", "动漫/插画"
	anime.CutoffSuspect, anime.SharpenHFRatio, anime.FlagLowNoiseAsAI = 0.60, 2.0, false
	anime.Note = "插画天生平涂、低噪、硬边:不因低噪点判 AI,放宽最近邻/锐化阈值;有效分辨率仍以线稿边缘高频衡量。"
	game := basePhoto()
	game.Name, game.Desc = "game", "游戏截图"
	game.CutoffSuspect, game.SharpenHFRatio, game.FlagLowNoiseAsAI = 0.62, 2.0, false
	game.Note = "游戏画面常为引擎直出(低噪)、含锐利 UI 文字与平涂 HUD:放宽低噪/锐化/最近邻判定,避免把渲染特性误判为后期处理。"
	shot := basePhoto()
	shot.Name, shot.Desc = "screenshot", "屏幕截图/文档"
	shot.CutoffSuspect, shot.SharpenHFRatio, shot.FlagLowNoiseAsAI, shot.PeakProminence = 0.55, 2.4, false, 14.0
	shot.Note = "截图含大量纯色块与点阵文字:频域统计不可靠,仅作弱参考;重点看像素分级与最近邻/缩放栅格。"
	return map[string]Config{"photo": photo, "anime": anime, "game": game, "screenshot": shot}
}

var tierMinWidth = map[string]int{"8K": 7680, "4K": 3840, "2K": 2560, "1K": 1920, "720P": 1280}
var tierOrder = []string{"8K", "4K", "2K", "1K", "720P"}

const tierTolerance = 0.97

// 已知编辑/超分/生成软件关键词(小写匹配)
var softwareKeywords = []struct{ kw, desc string }{
	{"topaz", "Topaz (Gigapixel/Photo AI,AI 放大)"},
	{"gigapixel", "Topaz Gigapixel AI (AI 放大)"},
	{"real-esrgan", "Real-ESRGAN (AI 超分)"},
	{"realesrgan", "Real-ESRGAN (AI 超分)"},
	{"esrgan", "ESRGAN / Real-ESRGAN (AI 超分)"},
	{"waifu2x", "waifu2x (AI 超分)"},
	{"upscayl", "Upscayl (AI 超分)"},
	{"remini", "Remini (AI 修复/放大)"},
	{"swinir", "SwinIR (AI 超分)"},
	{"srcnn", "SRCNN (AI 超分)"},
	{"photoshop", "Adobe Photoshop (编辑)"},
	{"lightroom", "Adobe Lightroom (编辑)"},
	{"gimp", "GIMP (编辑)"},
	{"snapseed", "Snapseed (移动端编辑)"},
	{"meitu", "美图 (移动端美化)"},
	{"美图", "美图 (移动端美化)"},
	{"stable diffusion", "Stable Diffusion (AI 生成)"},
	{"comfyui", "ComfyUI (AI 生成/工作流)"},
	{"automatic1111", "AUTOMATIC1111 (AI 生成)"},
	{"midjourney", "Midjourney (AI 生成)"},
	{"dall-e", "DALL·E (AI 生成)"},
	{"dalle", "DALL·E (AI 生成)"},
	{"firefly", "Adobe Firefly (AI 生成)"},
}

// =============================================================================
// 结果类型(对外 JSON)
// =============================================================================

type Finding struct {
	Level string `json:"level"` // info | warn | bad
	Text  string `json:"text"`
}

type Channel struct {
	Cutoff *float64 `json:"cutoff"`
	EffPx  *float64 `json:"eff_px"`
	Noise  *float64 `json:"noise"`
}

type Chroma struct {
	LumaCutoff   *float64 `json:"luma_cutoff"`
	ChromaCutoff *float64 `json:"chroma_cutoff"`
	Ratio        *float64 `json:"chroma_luma_ratio"`
	Subsample    string   `json:"subsample"`
}

type CA struct {
	ShiftX    int     `json:"shift_x"`
	ShiftY    int     `json:"shift_y"`
	Magnitude float64 `json:"magnitude"`
	Present   bool    `json:"present"`
}

type Nearest struct {
	EqualFraction    float64  `json:"equal_fraction"`
	IsNN             bool     `json:"is_nn"`
	Factor           *float64 `json:"factor"`
	BlockConsistency float64  `json:"block_consistency"`
}

type Sharpen struct {
	OvershootRatio float64 `json:"overshoot_ratio"`
	Sharpened      bool    `json:"sharpened"`
}

type Effective struct {
	Cutoff         float64 `json:"cutoff"`
	UpscaleFactor  float64 `json:"upscale_factor"`
	UpscaleRounded float64 `json:"upscale_factor_rounded"`
	HFRatio        float64 `json:"hf_ratio"`
	Plateau        float64 `json:"plateau"`
	TailDropDB     float64 `json:"tail_drop_db"`
}

// Spectrum 供前端画白化径向频谱曲线。
type Spectrum struct {
	Centers  []float64 `json:"centers"`
	Whitened []float64 `json:"whitened"`
	Cutoff   float64   `json:"cutoff"`
}

type Metadata struct {
	Camera       string   `json:"camera"`
	Software     string   `json:"software"`
	SoftwareHits []string `json:"software_hits"`
	DateTime     string   `json:"datetime"`
	JPEGQuality  *float64 `json:"jpeg_quality"`
}

type ClaimCheck struct {
	Claim   string `json:"claim"`
	PixelOK bool   `json:"pixel_ok"`
	EffOK   bool   `json:"eff_ok"`
	Real    bool   `json:"real"`
}

// Result 是单张图片的完整检测结果。
type Result struct {
	File         string  `json:"file"`
	Format       string  `json:"format"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	Filesize     int64   `json:"filesize"`
	Megapixels   float64 `json:"megapixels"`
	Tier         string  `json:"tier"`
	Aspect       float64 `json:"aspect"`
	Preset       string  `json:"preset"`
	PresetDesc   string  `json:"preset_desc"`
	AutoNote     string  `json:"auto_note"`
	Verdict      string  `json:"verdict"` // OK | WARN | FAIL
	VerdictText  string  `json:"verdict_text"`
	VerdictColor string  `json:"verdict_color"`

	EffectiveResolutionPx *float64   `json:"effective_resolution_px"`
	IsNative              *bool      `json:"is_native"`
	AISuspect             bool       `json:"ai_super_resolution_suspect"`
	Interpolation         string     `json:"interpolation"`
	Effective             *Effective `json:"effective"`
	Spectrum              *Spectrum  `json:"spectrum"`

	Channels   map[string]Channel `json:"channels"`
	Chroma     Chroma             `json:"chroma"`
	CA         CA                 `json:"chromatic_aberration"`
	Nearest    Nearest            `json:"nearest_neighbor"`
	NoiseSigma *float64           `json:"noise_sigma"`
	Sharpen    Sharpen            `json:"sharpen"`
	Metadata   Metadata           `json:"metadata"`

	ProcessingChain []string    `json:"processing_chain"`
	Reasons         []string    `json:"reasons"`
	ClaimCheckOut   *ClaimCheck `json:"claim_check"`
	Findings        []Finding   `json:"findings"`
}

// =============================================================================
// 入口
// =============================================================================

// Analyze 解码并分析图片字节。preset 为 "" 或 "auto" 时自动识别内容类型;
// claim 为 ""/"4k"/"2k"... 时核验标称分级。filename/filesize 仅用于展示。
func Analyze(data []byte, filename string, preset, claim string) (*Result, error) {
	im, format, err := decode(data)
	if err != nil {
		return nil, err
	}
	w, h := im.w, im.h
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("图片尺寸异常(%dx%d)", w, h)
	}
	if w*h > maxPixels {
		return nil, fmt.Errorf("图片像素过多(%dx%d),请压缩到 5000 万像素以内再检测", w, h)
	}
	longSide := maxInt(w, h)

	presets := Presets()
	var cfg Config
	autoNote := ""
	if preset == "" || preset == "auto" {
		guessed := guessContentType(im)
		cfg = presets[guessed]
		autoNote = "自动识别为「" + cfg.Desc + "」"
	} else if c, ok := presets[preset]; ok {
		cfg = c
	} else {
		cfg = presets["photo"]
	}

	lum := fullLuminance(im)

	eff := estimateEffectiveResolution(lum, cfg)
	resample := detectResampling(lum, cfg)
	nn := detectNearestNeighbor(lum, cfg)
	noise := estimateNoise(centerCrop(lum, 1600))
	sharp := detectSharpening(centerCrop(lum, 1600), cfg)
	channels := perChannelAnalysis(im, cfg, longSide)
	chroma := chromaAnalysis(im, cfg)
	ca := chromaticAberration(im)
	md := extractMetadata(data, format)

	r := &Result{
		File:       filename,
		Format:     format,
		Width:      w,
		Height:     h,
		Filesize:   int64(len(data)),
		Megapixels: float64(w*h) / 1e6,
		Aspect:     ratio(w, h),
		Preset:     cfg.Name,
		PresetDesc: cfg.Desc,
		AutoNote:   autoNote,
		Channels:   channels,
		Chroma:     chroma,
		CA:         ca,
		Nearest:    nn,
		Sharpen:    sharp,
		Metadata:   md,
	}
	r.Tier = classifyTier(longSide)
	if noise != nil {
		r.NoiseSigma = noise
	}
	if eff != nil {
		r.Effective = &Effective{eff.cutoff, eff.upscale, eff.upscaleRounded, eff.hfRatio, eff.plateau, eff.tailDropDB}
		r.Spectrum = &Spectrum{Centers: eff.centers, Whitened: eff.whitened, Cutoff: eff.cutoff}
	}

	var findings []Finding
	var processing, reasons []string
	add := func(level, text string) { findings = append(findings, Finding{level, text}) }

	var effLong *float64
	var verdictNative *bool

	// ---- 有效分辨率与放大 ----
	if eff != nil {
		cutoff := eff.cutoff
		el := cutoff * float64(longSide)
		effLong = &el
		switch {
		case cutoff >= cfg.CutoffNative:
			t := true
			verdictNative = &t
			add("info", fmt.Sprintf("频域有效分辨率≈%.0fpx(截止 %.2f),接近实际像素,未见明显放大", el, cutoff))
		case cutoff < cfg.CutoffSuspect:
			f := false
			verdictNative = &f
			uf := eff.upscaleRounded
			add("bad", fmt.Sprintf("频域有效分辨率仅≈%.0fpx(截止 %.2f),远低于实际 %dpx", el, cutoff, longSide))
			reasons = append(reasons, fmt.Sprintf("高频信息缺失:真实细节只相当于约 %.0fpx,疑似由低分辨率放大约 %s 倍而来", el, pyFloat(uf)))
			processing = append(processing, fmt.Sprintf("放大插值(约 %sx)", pyFloat(uf)))
		default:
			add("warn", fmt.Sprintf("频域有效分辨率≈%.0fpx(截止 %.2f),介于原生与放大之间,存在轻度放大或较强压缩可能", el, cutoff))
		}
	}

	// ---- 最近邻 ----
	interpType := ""
	if nn.IsNN {
		interpType = "最近邻(Nearest)"
		factor := 2.0
		if nn.Factor != nil {
			factor = *nn.Factor
		}
		fac := "约 " + pyFloat(factor) + "x"
		add("bad", fmt.Sprintf("最近邻放大特征:相邻相等像素占比 %.0f%% %s", nn.EqualFraction*100, fac))
		processing = append(processing, "最近邻放大"+fac)
		f := false
		verdictNative = &f
		nnEff := float64(longSide) / factor
		if effLong == nil || nnEff < *effLong {
			effLong = &nnEff
		}
		reasons = append(reasons, fmt.Sprintf("最近邻放大:由约 %.0fpx 的低分辨率以最近邻方式放大 %s,仅是像素复制成块状阶梯,未增加任何真实细节", nnEff, fac))
	}

	// 栅格类证据仅在「有效分辨率确实偏低」时作为放大确证。
	effLow := eff != nil && eff.cutoff < cfg.CutoffNative
	for _, it := range resample.interpretation {
		tag := fmt.Sprintf("(周期%.1f/显著度%.0f×)", it.period, it.prominence)
		switch it.kind {
		case "JPEG":
			add("info", it.text+tag)
		case "UPSCALE_JPEG":
			if effLow {
				add("bad", it.text+tag)
				reasons = append(reasons, it.text)
				processing = append(processing, "源 JPEG 放大")
				f := false
				verdictNative = &f
			} else {
				add("warn", it.text+tag+";但有效分辨率正常,可能为渐变带状/纹理伪周期,非放大确证")
			}
		case "RESAMPLE":
			if !nn.IsNN {
				if effLow {
					add("warn", it.text+tag)
					processing = append(processing, "重采样放大")
				} else {
					add("info", it.text+tag+";有效分辨率正常,疑为纹理/带状伪周期")
				}
			}
		}
	}

	if verdictNative != nil && !*verdictNative && !nn.IsNN && interpType == "" {
		if sharp.Sharpened {
			interpType = "双三次/Lanczos 类(含振铃/锐化)"
		} else {
			interpType = "平滑插值(双线性/双三次类)"
		}
		processing = append(processing, interpType)
		add("warn", "放大方式推断:"+interpType)
	}

	// ---- 锐化 ----
	hfRatio := 0.0
	if eff != nil {
		hfRatio = eff.hfRatio
	}
	if sharp.Sharpened || hfRatio > cfg.SharpenHFRatio {
		var why []string
		if sharp.Sharpened {
			why = append(why, fmt.Sprintf("强边缘过冲比例 %.0f%%", sharp.OvershootRatio*100))
		}
		if hfRatio > cfg.SharpenHFRatio {
			why = append(why, fmt.Sprintf("高频能量相对中频抬升 %.2f×", hfRatio))
		}
		add("warn", "锐化/增强痕迹:"+strings.Join(why, ","))
		processing = append(processing, "锐化(USM/高频增强)")
		reasons = append(reasons, "画质经过锐化处理:边缘人为变锐,并非传感器原生清晰度(USM 过冲/高频抬升)")
	}

	// ---- 噪声 / AI 超分嫌疑 ----
	aiSuspect := false
	var aiReasons []string
	if noise != nil {
		if cfg.FlagLowNoiseAsAI && *noise < cfg.NoiseLow && eff != nil && eff.cutoff >= cfg.CutoffNative {
			aiSuspect = true
			aiReasons = append(aiReasons, fmt.Sprintf("噪声极低(σ≈%.2f)却边缘锐利", *noise))
		}
		add("info", fmt.Sprintf("噪声估计 σ≈%.2f(0~255)", *noise))
	}
	if eff != nil && eff.cutoff < cfg.CutoffSuspect && sharp.Sharpened {
		aiSuspect = true
		aiReasons = append(aiReasons, "放大后仍保持锐利边缘(普通插值会变糊)")
	}
	var srSoftware []string
	for _, s := range md.SoftwareHits {
		if strings.Contains(s, "超分") || strings.Contains(s, "放大") || strings.Contains(s, "生成") {
			srSoftware = append(srSoftware, s)
		}
	}
	if len(srSoftware) > 0 {
		aiSuspect = true
		aiReasons = append(aiReasons, "元数据含软件痕迹:"+strings.Join(srSoftware, ";"))
	}
	if aiSuspect {
		add("bad", "AI 超分/重建嫌疑:"+strings.Join(aiReasons, ";"))
		processing = append(processing, "疑似 AI 超分(GAN/扩散类,如 Real-ESRGAN/Topaz)")
		reasons = append(reasons, "疑似 AI 超分:清晰度由神经网络'脑补'生成,而非真实采集("+strings.Join(aiReasons, ";")+")")
	}

	// ---- 通道/色度/色差 ----
	var cutoffs []float64
	for _, c := range []string{"R", "G", "B"} {
		if channels[c].Cutoff != nil {
			cutoffs = append(cutoffs, *channels[c].Cutoff)
		}
	}
	if len(cutoffs) > 0 {
		var parts []string
		for _, c := range []string{"R", "G", "B"} {
			if channels[c].EffPx != nil {
				parts = append(parts, fmt.Sprintf("%s=%.0fpx", c, *channels[c].EffPx))
			}
		}
		add("info", "RGB 通道有效分辨率: "+strings.Join(parts, " / "))
		spread := maxFloat(cutoffs) - minFloat(cutoffs)
		if spread < 0.03 && eff != nil && eff.cutoff < cfg.CutoffSuspect {
			add("warn", "三通道高频高度一致且整体偏低 -> 与单一来源放大/合成相符")
		}
	}
	if chroma.Subsample != "" {
		if chroma.Ratio != nil && *chroma.Ratio < 0.6 && format != "JPEG" {
			add("warn", "色度分辨率明显低于亮度,但当前为无损格式 -> 源头很可能是 JPEG(经历过有损压缩)")
		} else {
			add("info", chroma.Subsample)
		}
	}
	if ca.Present {
		add("info", fmt.Sprintf("检测到镜头色差(CA): R/B 通道横向位移≈%.1fpx -> 与真实镜头成像相符", ca.Magnitude))
	} else {
		add("info", "未检测到明显镜头色差(CA):合成/截图/AI 图常见,单独不构成判据")
	}

	// ---- 元数据发现 ----
	if md.Camera != "" {
		add("info", "拍摄设备:"+md.Camera)
	}
	if md.Software != "" {
		add("info", "Software 字段:"+md.Software)
	}
	for _, s := range md.SoftwareHits {
		add("warn", "软件痕迹:"+s)
	}
	if md.JPEGQuality != nil {
		q := *md.JPEGQuality
		lvl, suffix := "info", "(高)"
		if q < 85 {
			lvl, suffix = "warn", "(中)"
		}
		if q < 60 {
			lvl, suffix = "bad", "(低,已明显损失)"
		}
		add(lvl, fmt.Sprintf("JPEG 估计质量≈%.0f/100", q)+suffix)
		if q < 60 {
			reasons = append(reasons, fmt.Sprintf("JPEG 质量偏低(≈%.0f),高频细节亦受压缩损失影响", q))
		}
	}

	// ---- 标称核验 ----
	if claim != "" {
		cu := strings.ToUpper(claim)
		if nominal, ok := tierMinWidth[cu]; ok {
			pixelOK := longSide >= int(float64(nominal)*tierTolerance)
			effOK := effLong == nil || *effLong >= float64(nominal)*0.72
			real := pixelOK && effOK && !(verdictNative != nil && !*verdictNative)
			r.ClaimCheckOut = &ClaimCheck{Claim: cu, PixelOK: pixelOK, EffOK: effOK, Real: real}
			if !pixelOK {
				reasons = prepend(reasons, fmt.Sprintf("像素不达标:长边仅 %dpx,未达 %s 标称 %dpx", longSide, cu, nominal))
			} else if !effOK {
				el := 0.0
				if effLong != nil {
					el = *effLong
				}
				reasons = prepend(reasons, fmt.Sprintf("像素达标但有效分辨率不足:真实细节≈%.0fpx,达不到 %s 应有的清晰度(疑似放大充数)", el, cu))
			}
		}
	}

	r.Interpolation = interpType
	r.IsNative = verdictNative
	r.AISuspect = aiSuspect
	r.EffectiveResolutionPx = effLong
	r.ProcessingChain = sortedUnique(processing)
	r.Reasons = reasons
	r.Findings = findings

	code, text, hex := verdictState(verdictNative, reasons, aiSuspect)
	r.Verdict, r.VerdictText, r.VerdictColor = code, text, hex
	return r, nil
}

// verdictState 返回 (代码, 文案, 颜色 hex)。
func verdictState(native *bool, reasons []string, aiSuspect bool) (string, string, string) {
	if native != nil && !*native {
		return "FAIL", "非原生 · 疑似放大/重建", "#c82d2d"
	}
	if len(reasons) > 0 || aiSuspect {
		return "WARN", "原生分辨率 · 画质经人为处理", "#c88200"
	}
	return "OK", "原生真实画质", "#1e9632"
}

func classifyTier(longSide int) string {
	for _, t := range tierOrder {
		if longSide >= int(float64(tierMinWidth[t])*tierTolerance) {
			return t
		}
	}
	return "低于720P"
}
