package inspect

import "math"

// fftRadix2 计算长度为 2 的幂的复数序列的就地 FFT。
// inverse=true 时做逆变换(不做 1/n 归一化,由调用方处理)。
func fftRadix2(x []complex128, inverse bool) {
	n := len(x)
	if n <= 1 {
		return
	}
	// 位反转置换
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := 2 * math.Pi / float64(length)
		if !inverse {
			ang = -ang
		}
		wl := complex(math.Cos(ang), math.Sin(ang))
		half := length >> 1
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for k := 0; k < half; k++ {
				u := x[i+k]
				v := x[i+k+half] * w
				x[i+k] = u + v
				x[i+k+half] = u - v
				w *= wl
			}
		}
	}
}

func isPow2(n int) bool { return n > 0 && n&(n-1) == 0 }

// dft 计算任意长度复数序列的离散傅里叶变换:
// 长度为 2 的幂时走 radix-2 快路径;否则用 Bluestein(chirp-z)化为卷积。
func dft(x []complex128) []complex128 {
	n := len(x)
	if n == 0 {
		return nil
	}
	if isPow2(n) {
		y := make([]complex128, n)
		copy(y, x)
		fftRadix2(y, false)
		return y
	}
	// Bluestein:DFT_k = w_k * conv(a, b)_k, w_k = exp(-i*pi*k^2/n)
	m := 1
	for m < 2*n-1 {
		m <<= 1
	}
	a := make([]complex128, m)
	b := make([]complex128, m)
	w := make([]complex128, n)
	for k := 0; k < n; k++ {
		ang := -math.Pi * float64((k*k)%(2*n)) / float64(n)
		w[k] = complex(math.Cos(ang), math.Sin(ang))
		a[k] = x[k] * w[k]
	}
	b[0] = complex(1, 0)
	for k := 1; k < n; k++ {
		ang := math.Pi * float64((k*k)%(2*n)) / float64(n)
		v := complex(math.Cos(ang), math.Sin(ang))
		b[k] = v
		b[m-k] = v
	}
	fftRadix2(a, false)
	fftRadix2(b, false)
	for i := 0; i < m; i++ {
		a[i] *= b[i]
	}
	fftRadix2(a, true)
	inv := complex(1.0/float64(m), 0)
	out := make([]complex128, n)
	for k := 0; k < n; k++ {
		out[k] = a[k] * inv * w[k]
	}
	return out
}

// rfftMag 返回实数序列 rfft 的幅度谱(0..n/2),长度 n/2+1,与 numpy.abs(rfft) 对齐。
func rfftMag(x []float64) []float64 {
	n := len(x)
	cx := make([]complex128, n)
	for i, v := range x {
		cx[i] = complex(v, 0)
	}
	f := dft(cx)
	half := n/2 + 1
	out := make([]float64, half)
	for i := 0; i < half; i++ {
		out[i] = math.Hypot(real(f[i]), imag(f[i]))
	}
	return out
}

// fft2Power 对实矩阵做 2D FFT 并返回功率谱 |F|^2(未做 fftshift,按原始频率索引排列)。
// 输入各维若为 2 的幂则整条链路走 radix-2,速度最快。
func fft2Power(re [][]float64) [][]float64 {
	h := len(re)
	if h == 0 {
		return nil
	}
	w := len(re[0])
	rows := make([][]complex128, h)
	for i := 0; i < h; i++ {
		row := make([]complex128, w)
		for j := 0; j < w; j++ {
			row[j] = complex(re[i][j], 0)
		}
		rows[i] = dft(row)
	}
	col := make([]complex128, h)
	for j := 0; j < w; j++ {
		for i := 0; i < h; i++ {
			col[i] = rows[i][j]
		}
		c := dft(col)
		for i := 0; i < h; i++ {
			rows[i][j] = c[i]
		}
	}
	power := make([][]float64, h)
	for i := 0; i < h; i++ {
		p := make([]float64, w)
		for j := 0; j < w; j++ {
			re := real(rows[i][j])
			im := imag(rows[i][j])
			p[j] = re*re + im*im
		}
		power[i] = p
	}
	return power
}
