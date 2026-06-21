package inspect

import (
	"sort"
	"strings"
)

// 标准 JPEG 亮度量化表(自然/行主序,JPEG Annex K)。
var stdLum = [64]int{
	16, 11, 10, 16, 24, 40, 51, 61,
	12, 12, 14, 19, 26, 58, 60, 55,
	14, 13, 16, 24, 40, 57, 69, 56,
	14, 17, 22, 29, 51, 87, 80, 62,
	18, 22, 37, 56, 68, 109, 103, 77,
	24, 35, 55, 64, 81, 104, 113, 92,
	49, 64, 78, 87, 103, 121, 120, 101,
	72, 92, 95, 98, 112, 100, 103, 99,
}

// zigzag[k] = 第 k 个 zigzag 位置对应的自然(行主序)索引。用于把 DQT 的文件顺序
// 还原成自然顺序,与 PIL img.quantization(已 de-zigzag)对齐。
var zigzag = [64]int{
	0, 1, 8, 16, 9, 2, 3, 10,
	17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34,
	27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36,
	29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46,
	53, 60, 61, 54, 47, 55, 62, 63,
}

func qualityFromTable(tbl [64]int) *float64 {
	var qs []float64
	for i := 0; i < 64; i++ {
		q := tbl[i]
		if q <= 0 {
			continue
		}
		scale := float64(q) * 100.0 / float64(stdLum[i])
		var quality float64
		if scale <= 100 {
			quality = (200.0 - scale) / 2.0
		} else {
			quality = 5000.0 / scale
		}
		if quality < 1 {
			quality = 1
		}
		if quality > 100 {
			quality = 100
		}
		qs = append(qs, quality)
	}
	if len(qs) == 0 {
		return nil
	}
	m := median(qs)
	return &m
}

// jpegQualityFromDQT 解析一个 DQT 段(可能含多张表),取表号 0(亮度)估计质量。
func jpegQualityFromDQT(payload []byte) *float64 {
	pos := 0
	for pos < len(payload) {
		pqTq := payload[pos]
		pq := pqTq >> 4
		tq := pqTq & 0x0F
		pos++
		var tbl [64]int
		if pq == 0 {
			if pos+64 > len(payload) {
				return nil
			}
			for k := 0; k < 64; k++ {
				tbl[zigzag[k]] = int(payload[pos+k])
			}
			pos += 64
		} else {
			if pos+128 > len(payload) {
				return nil
			}
			for k := 0; k < 64; k++ {
				tbl[zigzag[k]] = int(payload[pos+2*k])<<8 | int(payload[pos+2*k+1])
			}
			pos += 128
		}
		if tq == 0 {
			return qualityFromTable(tbl)
		}
	}
	return nil
}

// extractMetadata 从原始字节提取 JPEG 质量、EXIF(设备/软件/时间)与软件痕迹关键词。
func extractMetadata(data []byte, format string) Metadata {
	md := Metadata{}
	var blob strings.Builder
	switch format {
	case "JPEG":
		parseJPEGMeta(data, &md, &blob)
	case "PNG":
		parsePNGText(data, &blob)
	}
	text := strings.ToLower(blob.String() + " " + md.Camera + " " + md.Software + " " + md.DateTime)
	seen := map[string]bool{}
	var hits []string
	for _, kd := range softwareKeywords {
		if strings.Contains(text, kd.kw) && !seen[kd.desc] {
			seen[kd.desc] = true
			hits = append(hits, kd.desc)
		}
	}
	sort.Strings(hits)
	md.SoftwareHits = hits
	return md
}

func parseJPEGMeta(data []byte, md *Metadata, blob *strings.Builder) {
	n := len(data)
	if n < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return
	}
	i := 2
	for i+4 <= n {
		if data[i] != 0xFF {
			i++
			continue
		}
		marker := data[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		if marker == 0xD9 || marker == 0xDA { // EOI / SOS:之后是熵编码数据
			break
		}
		if marker >= 0xD0 && marker <= 0xD7 { // RSTn 无长度
			i += 2
			continue
		}
		if i+4 > n {
			break
		}
		segLen := int(data[i+2])<<8 | int(data[i+3])
		if segLen < 2 {
			break
		}
		start := i + 4
		end := i + 2 + segLen
		if end > n {
			end = n
		}
		payload := data[start:end]
		switch {
		case marker == 0xDB: // DQT
			if md.JPEGQuality == nil {
				if q := jpegQualityFromDQT(payload); q != nil {
					md.JPEGQuality = q
				}
			}
		case marker == 0xE1: // APP1: EXIF 或 XMP
			if len(payload) >= 6 && string(payload[:4]) == "Exif" {
				parseEXIF(payload[6:], md)
			}
			blob.Write(payload)
			blob.WriteByte(' ')
		case marker == 0xFE: // COM 注释
			blob.Write(payload)
			blob.WriteByte(' ')
		}
		i = i + 2 + segLen
	}
}

// parseEXIF 解析 TIFF/EXIF 块(b 从 TIFF header 起始,即跳过 "Exif\x00\x00")。
func parseEXIF(b []byte, md *Metadata) {
	if len(b) < 8 {
		return
	}
	var le bool
	switch {
	case b[0] == 'I' && b[1] == 'I':
		le = true
	case b[0] == 'M' && b[1] == 'M':
		le = false
	default:
		return
	}
	u16 := func(p int) int {
		if p < 0 || p+2 > len(b) {
			return 0
		}
		if le {
			return int(b[p]) | int(b[p+1])<<8
		}
		return int(b[p])<<8 | int(b[p+1])
	}
	u32 := func(p int) int {
		if p < 0 || p+4 > len(b) {
			return 0
		}
		if le {
			return int(b[p]) | int(b[p+1])<<8 | int(b[p+2])<<16 | int(b[p+3])<<24
		}
		return int(b[p])<<24 | int(b[p+1])<<16 | int(b[p+2])<<8 | int(b[p+3])
	}
	readASCII := func(off, count int) string {
		if off < 0 || count <= 0 || off+count > len(b) {
			return ""
		}
		return strings.TrimRight(string(b[off:off+count]), "\x00 ")
	}
	decodeStr := func(typ, count, valpos int) string {
		if typ != 2 || count <= 0 {
			return ""
		}
		off := valpos
		if count > 4 {
			off = u32(valpos)
		}
		return readASCII(off, count)
	}
	var make_, model_, software_, dt_, dto_ string
	exifPtr := 0
	walk := func(ifdOff int, want map[int]*string, ptr *int) {
		if ifdOff <= 0 || ifdOff+2 > len(b) {
			return
		}
		cnt := u16(ifdOff)
		p := ifdOff + 2
		for k := 0; k < cnt && p+12 <= len(b); k++ {
			tag := u16(p)
			typ := u16(p + 2)
			c := u32(p + 4)
			if dst, ok := want[tag]; ok {
				*dst = decodeStr(typ, c, p+8)
			}
			if tag == 0x8769 && ptr != nil {
				*ptr = u32(p + 8)
			}
			p += 12
		}
	}
	walk(u32(4), map[int]*string{0x010F: &make_, 0x0110: &model_, 0x0131: &software_, 0x0132: &dt_}, &exifPtr)
	if exifPtr > 0 {
		walk(exifPtr, map[int]*string{0x9003: &dto_}, nil)
	}
	cam := strings.TrimSpace(strings.TrimSpace(make_) + " " + strings.TrimSpace(model_))
	if cam != "" {
		md.Camera = cam
	}
	if software_ != "" {
		md.Software = software_
	}
	if dto_ != "" {
		md.DateTime = dto_
	} else if dt_ != "" {
		md.DateTime = dt_
	}
}

// parsePNGText 收集 PNG 文本块(tEXt/iTXt;zTXt 压缩内容忽略)用于软件关键词匹配。
func parsePNGText(data []byte, blob *strings.Builder) {
	if len(data) < 8 {
		return
	}
	i := 8 // 跳过 PNG 签名
	for i+8 <= len(data) {
		length := int(data[i])<<24 | int(data[i+1])<<16 | int(data[i+2])<<8 | int(data[i+3])
		typ := string(data[i+4 : i+8])
		start := i + 8
		end := start + length
		if length < 0 || end > len(data) {
			break
		}
		if typ == "tEXt" || typ == "iTXt" {
			blob.Write(data[start:end])
			blob.WriteByte(' ')
		}
		if typ == "IEND" {
			break
		}
		i = end + 4 // 跳过 CRC
	}
}
