package services

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
)

var (
	errBreakdownNoLayers      = fmt.Errorf("breakdown: no ingredient layers detected in the bitmap")
	errBreakdownLayerCount    = fmt.Errorf("breakdown: detected layer count does not match the ingredient list")
	errBreakdownBakedInText   = fmt.Errorf("breakdown: leftover in-image text")
	errBreakdownAnimalProtein = fmt.Errorf("breakdown: animal protein on a vegetarian/vegan dish")
)

type breakdownLayer struct {
	CentroidX, CentroidY int
	MinY, MaxY           int
	MinX, MaxX           int
	Area                 int
}

type breakdownComponent struct {
	minX, minY, maxX, maxY int
	area                   int
	cx, cy                 int
}

func dietaryForbidsAnimalProtein(tags []string) bool {
	for _, t := range tags {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "vegetarian", "vegan":
			return true
		}
	}
	return false
}

func sampleCornerBackground(img *image.RGBA) color.RGBA {
	b := img.Bounds()
	pts := [][2]int{
		{b.Min.X + 2, b.Min.Y + 2},
		{b.Max.X - 3, b.Min.Y + 2},
		{b.Min.X + 2, b.Max.Y - 3},
		{b.Max.X - 3, b.Max.Y - 3},
	}
	var r, g, bl, n int
	for _, p := range pts {
		c := img.RGBAAt(p[0], p[1])
		r += int(c.R)
		g += int(c.G)
		bl += int(c.B)
		n++
	}
	if n == 0 {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	return color.RGBA{R: uint8(r / n), G: uint8(g / n), B: uint8(bl / n), A: 255}
}

func colorDist(a, b color.RGBA) int {
	dr := int(a.R) - int(b.R)
	dg := int(a.G) - int(b.G)
	db := int(a.B) - int(b.B)
	if dr < 0 {
		dr = -dr
	}
	if dg < 0 {
		dg = -dg
	}
	if db < 0 {
		db = -db
	}
	return dr + dg + db
}

func luminance(c color.RGBA) float64 {
	return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
}

func isBackgroundPixel(c, bg color.RGBA) bool {
	if c.R > 235 && c.G > 235 && c.B > 230 {
		return true
	}
	return colorDist(c, bg) < 48
}

func rgbHSV(c color.RGBA) (h, s, v float64) {
	r := float64(c.R) / 255
	g := float64(c.G) / 255
	b := float64(c.B) / 255
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	v = max
	d := max - min
	if max == 0 {
		return 0, 0, v
	}
	s = d / max
	if d == 0 {
		return 0, s, v
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	return h, s, v
}

func fillRectRGBA(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func inkMask(img *image.RGBA, bg color.RGBA) []bool {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	mask := make([]bool, w*h)
	bgLum := luminance(bg)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if isBackgroundPixel(c, bg) {
				continue
			}
			lum := luminance(c)
			_, sat, _ := rgbHSV(c)
			// Studio titles are dark, desaturated glyphs. Chromatic food
			// layers (kale, quinoa, chicken) must not be treated as ink.
			if sat < 0.20 && lum < 150 && bgLum-lum >= 50 {
				mask[y*w+x] = true
			}
		}
	}
	return mask
}

func connectedComponents(mask []bool, w, h int) []breakdownComponent {
	seen := make([]bool, len(mask))
	var out []breakdownComponent
	stack := make([]int, 0, 64)
	for i, on := range mask {
		if !on || seen[i] {
			continue
		}
		stack = stack[:0]
		stack = append(stack, i)
		seen[i] = true
		minX, minY := w, h
		maxX, maxY := 0, 0
		area := 0
		sumX, sumY := 0, 0
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := p%w, p/w
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
			area++
			sumX += x
			sumY += y
			for _, n := range []int{p - 1, p + 1, p - w, p + w} {
				if n < 0 || n >= len(mask) || seen[n] || !mask[n] {
					continue
				}
				nx := n % w
				// prevent wrap on left/right edges
				if (n == p-1 && nx == w-1) || (n == p+1 && nx == 0) {
					continue
				}
				seen[n] = true
				stack = append(stack, n)
			}
		}
		if area < 8 {
			continue
		}
		out = append(out, breakdownComponent{
			minX: minX, minY: minY, maxX: maxX + 1, maxY: maxY + 1,
			area: area, cx: sumX / area, cy: sumY / area,
		})
	}
	return out
}

func isTextLikeComponent(cc breakdownComponent, w, h int) bool {
	bw := cc.maxX - cc.minX
	bh := cc.maxY - cc.minY
	if bh < 4 || bw < 2 {
		return false
	}
	if bh > h/10 {
		return false
	}
	if cc.area > (w*h)/80 {
		return false
	}
	if bw > w/5 && bh < h/12 {
		return true
	}
	if cc.area <= 1100 && bh < h/14 {
		return true
	}
	return false
}

func textLineClusters(ccs []breakdownComponent, w, h int) [][]breakdownComponent {
	var glyphs []breakdownComponent
	for _, cc := range ccs {
		if isTextLikeComponent(cc, w, h) {
			glyphs = append(glyphs, cc)
		}
	}
	if len(glyphs) == 0 {
		return nil
	}
	sort.Slice(glyphs, func(i, j int) bool { return glyphs[i].cy < glyphs[j].cy })
	var clusters [][]breakdownComponent
	current := []breakdownComponent{glyphs[0]}
	band := h / 28
	if band < 8 {
		band = 8
	}
	anchor := glyphs[0].cy
	for _, g := range glyphs[1:] {
		if g.cy-anchor <= band {
			current = append(current, g)
			continue
		}
		clusters = append(clusters, current)
		current = []breakdownComponent{g}
		anchor = g.cy
	}
	clusters = append(clusters, current)

	var lines [][]breakdownComponent
	for _, cl := range clusters {
		if len(cl) >= 3 {
			lines = append(lines, cl)
			continue
		}
		// A single wide short banner (one connected title word).
		if len(cl) == 1 {
			bw := cl[0].maxX - cl[0].minX
			bh := cl[0].maxY - cl[0].minY
			if bw > w/5 && bh < h/12 {
				lines = append(lines, cl)
			}
		}
	}
	return lines
}

func wipeComponents(img *image.RGBA, comps []breakdownComponent, bg color.RGBA, pad int) {
	for _, cc := range comps {
		fillRectRGBA(img, cc.minX-pad, cc.minY-pad, cc.maxX+pad, cc.maxY+pad, bg)
	}
}

// stripBakedInBreakdownText paints out model-drawn captions and title bands.
// Side margins are reserved for our overlay; leftover studio titles (the
// "BRIGHT APERITIFI COCKTAIL" class) are detected as text-line clusters.
func stripBakedInBreakdownText(img *image.RGBA) {
	bg := sampleCornerBackground(img)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	mask := inkMask(img, bg)
	ccs := connectedComponents(mask, w, h)
	for _, line := range textLineClusters(ccs, w, h) {
		wipeComponents(img, line, bg, 3)
	}

	side := int(float64(w) * breakdownSideBandFrac)
	fillRectRGBA(img, 0, 0, side, h, bg)
	fillRectRGBA(img, w-side, 0, w, h, bg)
}

// breakdownHasBakedInText reports leftover caption/title clusters after strip.
func breakdownHasBakedInText(img *image.RGBA) bool {
	bg := sampleCornerBackground(img)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	side := int(float64(w) * breakdownSideBandFrac)
	mask := inkMask(img, bg)
	ccs := connectedComponents(mask, w, h)
	var remaining []breakdownComponent
	for _, cc := range ccs {
		if !isTextLikeComponent(cc, w, h) {
			continue
		}
		// Ignore anything still sitting in the wiped side bands.
		if cc.maxX <= side || cc.minX >= w-side {
			continue
		}
		remaining = append(remaining, cc)
	}
	return len(textLineClusters(remaining, w, h)) > 0
}

func isMeatLikePixel(c color.RGBA) bool {
	h, s, v := rgbHSV(c)
	if v < 0.25 || v > 0.92 || s < 0.14 {
		return false
	}
	// Candy/tomato red is too saturated to be cooked meat.
	if s > 0.72 && (h <= 10 || h >= 350) {
		return false
	}
	if h > 22 && h < 345 {
		return false
	}
	return s <= 0.70
}

// breakdownHasAnimalProtein reports a meat/poultry/fish-colored mass in the
// center stack. Used as a fail-closed guard for vegetarian/vegan items.
func breakdownHasAnimalProtein(img *image.RGBA) bool {
	bg := sampleCornerBackground(img)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	x0 := int(float64(w) * breakdownSideBandFrac)
	x1 := w - x0
	if x1-x0 < 8 {
		return false
	}
	meat := 0
	stack := 0
	for y := 0; y < h; y++ {
		for x := x0; x < x1; x++ {
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if isBackgroundPixel(c, bg) {
				continue
			}
			stack++
			if isMeatLikePixel(c) {
				meat++
			}
		}
	}
	if meat < 1500 {
		return false
	}
	if stack == 0 {
		return false
	}
	return float64(meat)/float64(stack) >= 0.025
}

func detectBreakdownLayers(img *image.RGBA, want int) ([]breakdownLayer, error) {
	if want < 1 {
		return nil, errBreakdownNoLayers
	}
	bg := sampleCornerBackground(img)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	x0 := int(float64(w) * breakdownSideBandFrac)
	x1 := w - x0
	if x1-x0 < 8 {
		return nil, errBreakdownNoLayers
	}

	occ := make([]float64, h)
	span := float64(x1 - x0)
	for y := 0; y < h; y++ {
		n := 0
		for x := x0; x < x1; x++ {
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if !isBackgroundPixel(c, bg) {
				n++
			}
		}
		occ[y] = float64(n) / span
	}

	const thresh = 0.06
	type run struct{ y0, y1 int }
	var runs []run
	in := false
	start := 0
	for y := 0; y < h; y++ {
		if occ[y] >= thresh {
			if !in {
				in = true
				start = y
			}
			continue
		}
		if in {
			in = false
			if y-start >= 5 {
				runs = append(runs, run{start, y})
			}
		}
	}
	if in && h-start >= 5 {
		runs = append(runs, run{start, h})
	}

	// Merge runs separated by a hairline gap (anti-aliasing, not a new layer).
	merged := make([]run, 0, len(runs))
	for _, r := range runs {
		if len(merged) == 0 {
			merged = append(merged, r)
			continue
		}
		prev := &merged[len(merged)-1]
		if r.y0-prev.y1 <= 4 {
			prev.y1 = r.y1
			continue
		}
		merged = append(merged, r)
	}

	layers := make([]breakdownLayer, 0, len(merged))
	for _, r := range merged {
		var sumX, sumY, area int
		minX, maxX := x1, x0
		for y := r.y0; y < r.y1; y++ {
			for x := x0; x < x1; x++ {
				c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
				if isBackgroundPixel(c, bg) {
					continue
				}
				area++
				sumX += x
				sumY += y
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
			}
		}
		if area < 40 {
			continue
		}
		layers = append(layers, breakdownLayer{
			CentroidX: sumX / area,
			CentroidY: sumY / area,
			MinY:      r.y0,
			MaxY:      r.y1,
			MinX:      minX,
			MaxX:      maxX + 1,
			Area:      area,
		})
	}
	if len(layers) == 0 {
		return nil, errBreakdownNoLayers
	}

	// Bottom layer first so it matches listed-order labels.
	sort.Slice(layers, func(i, j int) bool { return layers[i].CentroidY > layers[j].CentroidY })

	if want == 1 {
		return []breakdownLayer{unionBreakdownLayers(layers)}, nil
	}
	if len(layers) > want {
		sort.Slice(layers, func(i, j int) bool { return layers[i].Area > layers[j].Area })
		layers = layers[:want]
		sort.Slice(layers, func(i, j int) bool { return layers[i].CentroidY > layers[j].CentroidY })
		return layers, nil
	}
	if len(layers) != want {
		return nil, fmt.Errorf("%w: got %d want %d", errBreakdownLayerCount, len(layers), want)
	}
	return layers, nil
}

func unionBreakdownLayers(layers []breakdownLayer) breakdownLayer {
	u := layers[0]
	sumX := 0
	sumY := 0
	area := 0
	for _, l := range layers {
		if l.MinY < u.MinY {
			u.MinY = l.MinY
		}
		if l.MaxY > u.MaxY {
			u.MaxY = l.MaxY
		}
		if l.MinX < u.MinX {
			u.MinX = l.MinX
		}
		if l.MaxX > u.MaxX {
			u.MaxX = l.MaxX
		}
		sumX += l.CentroidX * l.Area
		sumY += l.CentroidY * l.Area
		area += l.Area
	}
	if area > 0 {
		u.CentroidX = sumX / area
		u.CentroidY = sumY / area
		u.Area = area
	}
	return u
}
