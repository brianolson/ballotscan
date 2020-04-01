package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

func maybeFail(err error, format string, args ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func pxy(it *image.YCbCr, x, y int) {
	fmt.Printf("(%d,%d) Y=%d, (%d,%d) CrCb=%d\n", x, y, it.YOffset(x, y), x, y, it.COffset(x, y))
}

func yHistogram(it *image.YCbCr) []uint {
	out := make([]uint, 256)
	for y := 0; y < it.Rect.Max.Y; y++ {
		for x := 0; x < it.Rect.Max.X; x++ {
			yv := it.Y[(it.YStride*y)+x]
			out[yv]++
		}
	}
	return out
}

func generalBrightnessHistogram(im image.Image) []uint {
	bounds := im.Bounds()
	out := make([]uint, 256)
	for y := 0; y < bounds.Max.Y; y++ {
		for x := 0; x < bounds.Max.X; x++ {
			c := im.At(x, y)
			r, g, b, _ := c.RGBA()
			yv, _, _ := color.RGBToYCbCr(
				uint8(r>>8),
				uint8(g>>8),
				uint8(b>>8),
			)
			out[yv]++
		}
	}
	return out
}

// https://en.wikipedia.org/wiki/Otsu%27s_method
func otsuThreshold(hist []uint) uint8 {
	sumB := uint(0)
	wB := uint(0)
	max := 0.0
	total := uint(0)
	sum1 := uint(0)
	best := 0
	for i, hv := range hist {
		total += hv
		sum1 += uint(i) * hv
	}
	for i := 1; i < 256; i++ {
		if wB > 0 && total > wB {
			wF := total - wB
			mF := float64(sum1-sumB) / float64(wF)
			fwB := float64(wB)
			fwF := float64(wF)
			fsumB := float64(sumB)
			val := fwB * fwF * ((fsumB / fwB) - mF) * ((fsumB / fwB) - mF)
			if val >= max {
				best = i
				max = val
			}
		}
		wB += hist[i]
		sumB += uint(i) * hist[i]
	}
	return uint8(best)
}

//const threshold = 128
const darkPxCountThreshold = 4

// Search the Y compoment of YCbCr for a left edge
func yLeftLineFind(it *image.YCbCr, ySeekCenter int, threshold uint8) (edgeX int) {
	darkPxCount := 0
	leftEdge := 0
	rightEdge := 10
	for y := ySeekCenter - 1; y < ySeekCenter+2; y++ {
		for x := leftEdge; x < rightEdge; x++ {
			if it.Y[(it.YStride*y)+x] < threshold {
				darkPxCount++
			}
		}
	}
	for rightEdge < it.Rect.Max.X && darkPxCount < darkPxCountThreshold {
		for y := ySeekCenter - 1; y < ySeekCenter+2; y++ {
			x := leftEdge
			if it.Y[(it.YStride*y)+x] < threshold {
				darkPxCount--
			}
			x = rightEdge
			if it.Y[(it.YStride*y)+x] < threshold {
				darkPxCount++
			}
		}
		leftEdge++
		rightEdge++
	}
	return rightEdge - 1
}

func yTopLineFind(it *image.YCbCr, xSeekCenter int, threshold uint8) (edgeY int) {
	darkPxCount := 0
	topEdge := 0
	bottomEdge := 10
	for y := topEdge; y < bottomEdge; y++ {
		for x := xSeekCenter - 1; x < xSeekCenter+2; x++ {
			if it.Y[(it.YStride*y)+x] < threshold {
				darkPxCount++
			}
		}
	}
	for bottomEdge < it.Rect.Max.Y && darkPxCount < darkPxCountThreshold {
		for x := xSeekCenter - 1; x < xSeekCenter+2; x++ {
			y := topEdge
			if it.Y[(it.YStride*y)+x] < threshold {
				darkPxCount--
			}
			y = bottomEdge
			if it.Y[(it.YStride*y)+x] < threshold {
				darkPxCount++
			}
		}
		topEdge++
		bottomEdge++
	}
	return bottomEdge - 1
}

func main() {
	origname := "resources/testdata/20200330/a.png"
	r, err := os.Open(origname)
	maybeFail(err, "%s: %s", origname, err)
	orig, format, err := image.Decode(r)
	maybeFail(err, "%s: %s", origname, err)
	r.Close()
	fmt.Printf("orig %s %T\n", format, orig)
	// ohist := generalBrightnessHistogram(orig)
	// for i, v := range ohist {
	// 	fmt.Printf("hist[%3d] %6d\n", i, v)
	// }
	// os.Exit(0)
	fname := "resources/testdata/20200330/scan_20200330_102310_1.jpg"
	r, err = os.Open(fname)
	maybeFail(err, "%s: %s", fname, err)
	im, format, err := image.Decode(r)
	maybeFail(err, "%s: %s", fname, err)
	r.Close()
	fmt.Printf("im %s %T\n", format, im)
	switch it := im.(type) {
	case *image.YCbCr:
		fmt.Printf("it YStride %d CStride %d SubsampleRatio %v Rect %v\n", it.YStride, it.CStride, it.SubsampleRatio, it.Rect)
		pxy(it, 0, 0)
		pxy(it, 1, 0)
		pxy(it, 2, 0)
		pxy(it, 0, 1)
		pxy(it, 0, 2)
		pxy(it, 50, 50)
		pxy(it, it.Rect.Max.X-1, it.Rect.Max.Y-1)
		//fmt.Printf("(50,50) Y=%d, (50,50) CrCb=%d\n", it.COffset(50, 50), it.YOffset(50, 50))
		//fmt.Printf("(%d,%d) Y=%d, (%d,%d) CrCb=%d\n", it.Rect.Max.X-1, it.Rect.Max.Y-1, it.COffset(it.Rect.Max.X-1, it.Rect.Max.Y-1), it.Rect.Max.X-1, it.Rect.Max.Y-1, it.YOffset(it.Rect.Max.X-1, it.Rect.Max.Y-1))

		hist := yHistogram(it)
		threshold := otsuThreshold(hist)
		fmt.Printf("Otsu threshold %d\n", threshold)
		if false {
			for i, v := range hist {
				fmt.Printf("hist[%3d] %6d\n", i, v)
			}
		}
		misscount := 0
		hitcount := 0
		for y := 100; y < it.Rect.Max.Y-100; y += 50 {
			xle := yLeftLineFind(it, y, threshold)
			if xle < it.Rect.Max.X/2 {
				fmt.Printf("[%d,%d]\n", xle, y)
				hitcount++
			} else {
				misscount++
			}
		}
		fmt.Printf("left line %d hit %d miss\n", hitcount, misscount)
		misscount = 0
		hitcount = 0
		for x := 100; x < it.Rect.Max.X-100; x += 50 {
			yte := yTopLineFind(it, x, threshold)
			if yte < it.Rect.Max.Y/2 {
				fmt.Printf("[%d,%d]\n", x, yte)
				hitcount++
			} else {
				misscount++
			}
		}
		fmt.Printf("top line %d hit %d miss\n", hitcount, misscount)
	default:
	}
}
