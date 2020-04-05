package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"math"
	"os"
)

func maybeFail(err error, format string, args ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

// func pxy(it *image.YCbCr, x, y int) {
// 	fmt.Printf("(%d,%d) Y=%d, (%d,%d) CrCb=%d\n", x, y, it.YOffset(x, y), x, y, it.COffset(x, y))
// }

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

// https://en.wikipedia.org/wiki/Simple_linear_regression#Fitting_the_regression_line
func ordinaryLeastSquares(points []point) (slope, intercept float64) {
	xsum := int64(0)
	ysum := int64(0)
	for _, pt := range points {
		xsum += int64(pt.x)
		ysum += int64(pt.y)
	}
	xavg := float64(xsum) / float64(len(points))
	yavg := float64(ysum) / float64(len(points))
	N := 0.0
	D := 0.0
	for _, pt := range points {
		dx := (float64(pt.x) - xavg)
		N += dx * (float64(pt.y) - yavg)
		D += dx * dx
	}
	slope = N / D
	intercept = yavg - (slope * xavg)
	return
}

// https://en.wikipedia.org/wiki/Distance_from_a_point_to_a_line
func pointLineDistance(slope, intercept float64, x, y int) float64 {
	// ax + by + c = 0
	// (x0, y0)
	// abs(a*x0 + b*y0 + c)/sqrt(a*a + b*b)
	// y = slope*x + intercept
	// a = slope
	// b = -1
	// c = intercept
	return math.Abs((slope*float64(x))+(-1.0*float64(y))+intercept) / math.Sqrt((slope*slope)+1)
}

type point struct {
	x int
	y int
}

type transform struct {
	orig    point
	scale   float64
	rotRads float64
	costh   float64
	sinth   float64
	dest    point
}

func newTransform(origTopLeft, origTopRight, destTopLeft, destTopRight point) transform {
	dy := destTopRight.y - destTopLeft.y
	dx := destTopRight.x - destTopLeft.x
	rotRads := math.Atan2(float64(dy), float64(dx))
	actualTopLineLengthPx := math.Sqrt(float64((dx * dx) + (dy * dy)))
	ody := origTopRight.y - origTopLeft.y
	odx := origTopRight.x - origTopLeft.x
	origTopLineLength := math.Sqrt(float64((odx * odx) + (ody * ody)))
	scale := actualTopLineLengthPx / origTopLineLength
	costh := math.Cos(rotRads)
	sinth := math.Sin(rotRads)
	return transform{
		orig:    origTopLeft,
		scale:   scale,
		rotRads: rotRads,
		costh:   costh,
		sinth:   sinth,
		dest:    destTopLeft,
	}
}

func (t transform) transform(origx, origy int) (x, y int) {
	// TODO: compose this into a transform matrix
	x = origx - t.orig.x
	y = origy - t.orig.y
	nx := float64(x) * t.scale
	ny := float64(y) * t.scale

	x2 := (nx * t.costh) - (ny * t.sinth)
	y2 := (nx * t.sinth) + (ny * t.costh)

	x = int(x2) + t.dest.x
	y = int(y2) + t.dest.y
	return
}

type Scanner struct {
	bj BubblesJson

	orig         image.Image
	origPxPerPt  float64
	origTopLeft  point
	origTopRight point
}

func (s *Scanner) readBubblesJson(path string) error {
	fin, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fin.Close()
	jd := json.NewDecoder(fin)
	return jd.Decode(&s.bj)
}

func (s *Scanner) readOrigImage(origname string) error {
	r, err := os.Open(origname)
	maybeFail(err, "%s: %s", origname, err)
	orig, format, err := image.Decode(r)
	maybeFail(err, "%s: %s", origname, err)
	defer r.Close()
	s.orig = orig
	orect := orig.Bounds()
	if orect.Min.X != 0 || orect.Min.Y != 0 {
		return fmt.Errorf("nonzero origin for original pic. WAT?\n")
	}
	fmt.Printf("orig %s %T %v\n", format, orig, orect)
	origPxPerPtX := float64(orect.Max.X-orect.Min.X) / s.bj.DrawSettings.PageSize[0]
	origPxPerPtY := float64(orect.Max.Y-orect.Min.Y) / s.bj.DrawSettings.PageSize[1]
	if math.Abs((origPxPerPtY/origPxPerPtX)-1) > 0.01 {
		return fmt.Errorf("orig scale not square: mx = %f, my = %f\n", origPxPerPtX, origPxPerPtY)
	}
	s.origPxPerPt = (origPxPerPtX + origPxPerPtY) / 2.0
	s.origTopLeft = point{
		x: int(s.bj.DrawSettings.PageMargin * s.origPxPerPt),
		y: int(s.bj.DrawSettings.PageMargin * s.origPxPerPt),
	}
	s.origTopRight = point{
		x: int((s.bj.DrawSettings.PageSize[0] - s.bj.DrawSettings.PageMargin) * s.origPxPerPt),
		y: int(s.bj.DrawSettings.PageMargin * s.origPxPerPt),
	}
	fmt.Printf("top line orig (%d,%d)-(%d,%d)\n", s.origTopLeft.x, s.origTopLeft.y, s.origTopRight.x, s.origTopRight.y)
	return nil
}

func (s *Scanner) readScannedImage(fname string) error {
	r, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer r.Close()
	im, format, err := image.Decode(r)
	if err != nil {
		return err
	}
	//fmt.Printf("im %s %T %v\n", format, im, im.Bounds())
	switch it := im.(type) {
	case *image.YCbCr:
		return s.processYCbCr(it)
	default:
		return fmt.Errorf("unknown image type %s %T", format, im)
	}
}

// For some (x,y) point in the original image, return (x,y) in the scanned image
func (s *Scanner) pointOrigToScanned(origx, origy int, topLeft, topRight point) (x, y int, err error) {
	dy := topRight.y - topLeft.y
	dx := topRight.x - topLeft.x
	rotRads := math.Atan2(float64(dy), float64(dx))
	actualTopLineLengthPx := math.Sqrt(float64((dx * dx) + (dy * dy)))
	ody := s.origTopRight.y - s.origTopLeft.y
	odx := s.origTopRight.x - s.origTopLeft.x
	origTopLineLength := math.Sqrt(float64((odx * odx) + (ody * ody)))
	scale := actualTopLineLengthPx / origTopLineLength
	fmt.Printf("rotate %f radians, scale %f\n", rotRads, scale)

	// x,y in orig frame
	x = origx - s.origTopLeft.x
	y = origy - s.origTopLeft.y
	nx := float64(x) * scale
	ny := float64(y) * scale

	costh := math.Cos(rotRads)
	sinth := math.Sin(rotRads)
	x2 := (nx * costh) - (ny * sinth)
	y2 := (nx * sinth) + (ny * costh)

	x = int(x2) + topLeft.x
	y = int(y2) + topLeft.y
	return
}

func (s *Scanner) processYCbCr(it *image.YCbCr) error {
	if it.Rect.Min.X != 0 || it.Rect.Min.Y != 0 {
		return fmt.Errorf("image origin not 0,0 but %d,%d", it.Rect.Min.X, it.Rect.Min.Y)
	}
	fmt.Printf("it YStride %d CStride %d SubsampleRatio %v Rect %v\n", it.YStride, it.CStride, it.SubsampleRatio, it.Rect)
	// pxy(it, 0, 0)
	// pxy(it, 1, 0)
	// pxy(it, 2, 0)
	// pxy(it, 0, 1)
	// pxy(it, 0, 2)
	// pxy(it, 50, 50)
	// pxy(it, it.Rect.Max.X-1, it.Rect.Max.Y-1)
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
			//fmt.Printf("[%d,%d]\n", xle, y)
			hitcount++
		} else {
			misscount++
		}
	}
	fmt.Printf("left line %d hit %d miss\n", hitcount, misscount)
	misscount = 0
	hitcount = 0
	topPoints := make([]point, 0, 100)
	for x := 100; x < it.Rect.Max.X-100; x += 50 {
		yte := yTopLineFind(it, x, threshold)
		if yte < it.Rect.Max.Y/2 {
			//fmt.Printf("[%d,%d]\n", x, yte)
			topPoints = append(topPoints, point{x, yte})
			hitcount++
		} else {
			misscount++
		}
	}
	slope, intercept := ordinaryLeastSquares(topPoints)
	fmt.Printf("top line %d hit %d miss, slope=%f intercept=%f\n", hitcount, misscount, slope, intercept)
	worstd := 0.0
	for _, pt := range topPoints {
		d := pointLineDistance(slope, intercept, pt.x, pt.y)
		//fmt.Printf(" %0.2f", d)
		if d > worstd {
			worstd = d
		}
	}
	//fmt.Printf("\n")
	x := topPoints[0].x
	y := topPoints[0].y
	const step = 5
	for true {
		nx := x - step
		yte := yTopLineFind(it, nx, threshold)
		d := pointLineDistance(slope, intercept, nx, yte)
		if d > worstd {
			break
		}
		x = nx
		y = yte
	}
	topLeft := point{x, y}
	//topLeftX := x
	//topLeftY := y
	last := len(topPoints) - 1
	x = topPoints[last].x
	y = topPoints[last].y
	for true {
		nx := x + step
		yte := yTopLineFind(it, nx, threshold)
		d := pointLineDistance(slope, intercept, nx, yte)
		if d > worstd {
			break
		}
		x = nx
		y = yte
	}
	//topRightX := x
	//topRightY := y
	topRight := point{x, y}
	fmt.Printf("topleft (%d,%d) topright (%d,%d)\n", topLeft.x, topLeft.y, topRight.x, topRight.y)

	origToScanned := newTransform(s.origTopLeft, s.origTopRight, topLeft, topRight)
	// TODO: compase a 2d transformation matrix
	// translate(-topLeftX, -topLeftY)
	// rotate(-rotRads)
	// scale
	// dy := topRightY - topLeftY
	// dx := topRightX - topLeftX
	// rotRads := math.Atan2(float64(dy), float64(dx))
	// actualTopLineLengthPx := math.Sqrt(float64((dx * dx) + (dy * dy)))
	// ody := s.origTopRight.y - s.origTopLeft.y
	// odx := s.origTopRight.x - s.origTopLeft.x
	// origTopLineLength := math.Sqrt(float64((odx * odx) + (ody * ody)))
	// scale := actualTopLineLengthPx / origTopLineLength
	// fmt.Printf("rotate %f radians, scale %f\n", rotRads, scale)

	orect := s.orig.Bounds()
	oi := image.NewNRGBA(orect)
	for iy := orect.Min.Y; iy < orect.Max.Y; iy++ {
		// zero based coord
		zy := iy - orect.Min.Y
		for ix := orect.Min.X; ix < orect.Max.X; ix++ {
			zx := ix - orect.Min.X
			sx, sy := origToScanned.transform(zx, zy)
			//v := uint8((ix + iy) & 0x0ff)
			v := it.Y[(sy*it.YStride)+sx]
			pi := (zy * oi.Stride) + (zx * 4)
			oi.Pix[pi] = v      // R
			oi.Pix[pi+1] = v    // G
			oi.Pix[pi+2] = v    // B
			oi.Pix[pi+3] = 0xff // A
		}
	}
	imoutpath := "/tmp/oi.png"
	imout, err := os.Create(imoutpath)
	if err != nil {
		return fmt.Errorf("%s: %s", imoutpath, err)
	}
	defer imout.Close()
	err = png.Encode(imout, oi)
	return err
}

type DrawSettings struct {
	PageSize   []float64 `json:"pagesize"`
	PageMargin float64   `json:"pageMargin"`
	// TODO: lots of fields ignored
}

type BubblesJson struct {
	DrawSettings *DrawSettings            `json:"draw_settings"`
	Bubbles      []map[string]interface{} `json:"bubbles"`
}

func readBubbles(path string) (out *BubblesJson, err error) {
	fin, err := os.Open(path)
	if err != nil {
		return
	}
	out = new(BubblesJson)
	//var xo bubblesJson
	jd := json.NewDecoder(fin)
	err = jd.Decode(out)
	fin.Close()
	//out = &xo
	return
}

func main() {
	var err error
	var s Scanner
	bfname := "resources/testdata/20200403/bubbles.json"
	//ob, err := readBubbles(bfname)
	err = s.readBubblesJson(bfname)
	maybeFail(err, "%s: %s", bfname, err)
	fmt.Printf("ds %#v\n", s.bj.DrawSettings)
	origname := "resources/testdata/20200330/a.png"
	err = s.readOrigImage(origname)
	maybeFail(err, "%s: %s\n", origname, err)
	// ohist := generalBrightnessHistogram(orig)
	// for i, v := range ohist {
	// 	fmt.Printf("hist[%3d] %6d\n", i, v)
	// }
	// os.Exit(0)
	fname := "resources/testdata/20200330/scan_20200330_102310_1.jpg"
	err = s.readScannedImage(fname)
	maybeFail(err, "%s: %s\n", fname, err)
}
