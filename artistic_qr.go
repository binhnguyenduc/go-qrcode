package qrcode

import (
	"image"
	"image/gif"
	"math"

	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
)

// GifGenerator can generate a gif qr code
func GifGenerator(q *QRCode, g gif.GIF, size int) *gif.GIF {

	return &g
}

// ImageGenerator can generate a artistic qr code
func ImageGenerator(q *QRCode, g image.Image, size int) image.Image {
	bg := scale(g, size)
	// Minimum pixels (both width and height) required.
	realSize := q.symbol.size

	// Variable size support.
	if size < 0 {
		size = size * -1 * realSize
	}

	// Actual pixels available to draw the symbol. Automatically increase the
	// image size if it's not large enough.
	if size < realSize {
		size = realSize
	}
	// Size of each module drawn.
	pixelsPerModule := size / realSize

	// Center the symbol within the image.
	// offset := (size - realSize*pixelsPerModule) / 2
	bgTmp := image.NewRGBA(image.Rect(0, 0, pixelsPerModule*realSize, pixelsPerModule*realSize))
	bitmap := q.symbol.bitmap()
	for y, row := range bitmap {
		for x, v := range row {
			//if the point is belong to FinderPatterns,AlignmentPatterns,TimingPatterns,dont scale it
			var startX, startY, lenX, lenY int
			if q.getPointType(x, y) <= 0 {
				startX = x*pixelsPerModule + pixelsPerModule/4
				startY = y*pixelsPerModule + pixelsPerModule/4
				lenX = startX + pixelsPerModule - pixelsPerModule/2
				lenY = startY + pixelsPerModule - pixelsPerModule/2
			} else {
				startX = x * pixelsPerModule
				startY = y * pixelsPerModule
				lenX = startX + pixelsPerModule
				lenY = startY + pixelsPerModule
			}
			if v {
				for i := startX; i < lenX; i++ {
					for j := startY; j < lenY; j++ {
						bgTmp.Set(i, j, q.ForegroundColor)
						// g.Set(i, j, q.ForegroundColor)
					}
				}
			} else {
				for i := startX; i < lenX; i++ {
					for j := startY; j < lenY; j++ {
						bgTmp.Set(i, j, q.BackgroundColor)
						// g.Set(i, j, q.ForegroundColor)
					}
				}
			}
		}
	}
	if float64(size)/float64(bgTmp.Bounds().Dx()) > 1 {
		tmp := scale(bgTmp, size)
		draw.Draw(&bg, bg.Bounds(), &tmp, image.ZP, draw.Over)
	}
	return &bg
}

func scale(g image.Image, size int) image.RGBA {
	bg := image.NewRGBA(image.Rect(0, 0, size, size))
	transform := draw.CatmullRom
	tmp := newunits()
	tmp.sacle(float64(size)/float64(g.Bounds().Dx()), float64(size)/float64(g.Bounds().Dy()))
	martix := tmp.getAff3()
	transform.Transform(bg, martix,
		g, g.Bounds(), draw.Over, nil,
	)
	return *bg
}

type Uniterm struct { //一个单元项
	Coefficient float64 //系数
	Variable    string  //变量
}
type Point struct {
	X []Uniterm
	Y []Uniterm
}

func newunits() *Point {
	return &Point{
		X: []Uniterm{Uniterm{Coefficient: 1, Variable: "X0"}},
		Y: []Uniterm{Uniterm{Coefficient: 1, Variable: "Y0"}},
	}
}

func (p *Point) sacle(sx, sy float64) {
	for i := 0; i < len(p.X); i++ {
		p.X[i].Coefficient = p.X[i].Coefficient * sx
	}
	for i := 0; i < len(p.Y); i++ {
		p.Y[i].Coefficient = p.Y[i].Coefficient * sy
	}
}

func (p *Point) rotate(road, rx, ry float64) {
	rotate := (math.Pi / 180.0) * road
	c := math.Cos(rotate)
	s := math.Sin(rotate)
	x0, y0 := make([]Uniterm, len(p.X)), make([]Uniterm, len(p.Y))
	copy(x0, p.X)
	copy(y0, p.Y)
	// X
	p.X = nil
	p.Y = nil
	for i := 0; i < len(x0); i++ {
		p.X = append(p.X, Uniterm{x0[i].Coefficient * c, x0[i].Variable})
	}
	for i := 0; i < len(y0); i++ {
		p.X = append(p.X, Uniterm{y0[i].Coefficient * (-s), y0[i].Variable})
	}
	p.X = append(p.X, Uniterm{Coefficient: -c*rx + s*ry + rx})
	// Y
	for i := 0; i < len(x0); i++ {
		p.Y = append(p.Y, Uniterm{x0[i].Coefficient * s, x0[i].Variable})
	}
	for i := 0; i < len(y0); i++ {
		p.Y = append(p.Y, Uniterm{y0[i].Coefficient * c, y0[i].Variable})
	}
	p.Y = append(p.Y, Uniterm{Coefficient: -s*rx - c*ry + ry})
}

func (p *Point) translate(mx, my float64) {
	for i := 0; i < len(p.X); i++ {
		if p.X[i].Variable == "" {
			p.X[i].Coefficient += mx
			break
		}
		if i == len(p.X)-1 {
			p.X = append(p.X, Uniterm{Coefficient: mx})
			break
		}
	}
	for i := 0; i < len(p.Y); i++ {
		if p.Y[i].Variable == "" {
			p.Y[i].Coefficient += mx
			break
		}
		if i == len(p.Y)-1 {
			p.Y = append(p.Y, Uniterm{Coefficient: my})
			break
		}
	}
}

func (p *Point) getAff3() f64.Aff3 {
	t := [6]float64{}
	for i := 0; i < len(p.X); i++ {
		switch p.X[i].Variable {
		case "X0":
			t[0] += p.X[i].Coefficient
		case "Y0":
			t[1] += p.X[i].Coefficient
		default:
			t[2] += p.X[i].Coefficient
		}
	}
	for i := 0; i < len(p.Y); i++ {
		switch p.Y[i].Variable {
		case "X0":
			t[3] += p.Y[i].Coefficient
		case "Y0":
			t[4] += p.Y[i].Coefficient
		default:
			t[5] += p.Y[i].Coefficient
		}
	}
	return f64.Aff3(t)
}
