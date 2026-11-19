package theme

import (
	"image/color"
	"math"
)

// Turned is c with its hue turned by degrees round OKLCH's wheel, its
// lightness and chroma kept, the chroma eased only as far as sRGB needs, so
// text that reads on c reads on it as well.
func Turned(c color.Color, degrees float64) color.Color {
	r, g, b, _ := c.RGBA()
	l, a, bb := toOKLab(linear(r), linear(g), linear(b))
	chroma := math.Hypot(a, bb)
	angle := math.Atan2(bb, a) + degrees*math.Pi/180
	for ; chroma > 0; chroma -= chromaEase {
		if lr, lg, lb := fromOKLab(l, chroma*math.Cos(angle), chroma*math.Sin(angle)); inGamut(lr, lg, lb) {
			return toRGBA(lr, lg, lb)
		}
	}
	lr, lg, lb := fromOKLab(l, 0, 0)
	return toRGBA(lr, lg, lb)
}

// chromaEase is how far Turned eases the chroma at a time, till the turned
// colour is one sRGB holds.
const chromaEase = 0.002

// linear is a 16-bit sRGB channel, as color.Color's RGBA gives it, as linear
// light, from 0 to 1.
func linear(channel uint32) float64 {
	v := float64(channel) / 0xffff
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// encoded is a channel of linear light as sRGB encodes it, from 0 to 1.
func encoded(v float64) float64 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

// toOKLab is linear sRGB as OKLab, by Björn Ottosson's matrices.
func toOKLab(r, g, b float64) (l, a, bb float64) {
	lc := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	mc := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	sc := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return 0.2104542553*lc + 0.7936177850*mc - 0.0040720468*sc,
		1.9779984951*lc - 2.4285922050*mc + 0.4505937099*sc,
		0.0259040371*lc + 0.7827717662*mc - 0.8086757660*sc
}

// fromOKLab is OKLab as linear sRGB, which may fall outside 0 to 1.
func fromOKLab(l, a, bb float64) (r, g, b float64) {
	lc := cube(l + 0.3963377774*a + 0.2158037573*bb)
	mc := cube(l - 0.1055613458*a - 0.0638541728*bb)
	sc := cube(l - 0.0894841775*a - 1.2914855480*bb)
	return 4.0767416621*lc - 3.3077115913*mc + 0.2309699292*sc,
		-1.2684380046*lc + 2.6097574011*mc - 0.3413193965*sc,
		-0.0041960863*lc - 0.7034186147*mc + 1.7076147010*sc
}

// cube is v cubed.
func cube(v float64) float64 {
	return v * v * v
}

// gamutSlack is how far past 0 or 1 a channel may fall and still count as in
// sRGB, as the matrices round.
const gamutSlack = 1e-4

// inGamut reports whether linear sRGB is a colour sRGB holds.
func inGamut(channels ...float64) bool {
	for _, v := range channels {
		if v < -gamutSlack || v > 1+gamutSlack {
			return false
		}
	}
	return true
}

// toRGBA is linear sRGB as an 8-bit colour, each channel held to 0 to 1.
func toRGBA(r, g, b float64) color.RGBA {
	channel := func(v float64) uint8 {
		return uint8(math.Round(encoded(min(max(v, 0), 1)) * 0xff))
	}
	return color.RGBA{R: channel(r), G: channel(g), B: channel(b), A: 0xff}
}
