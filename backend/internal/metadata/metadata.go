package metadata

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/gobold"
)

//go:embed assets
var assets embed.FS

type NFTMetadata struct {
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Image        string              `json:"image"`
	Properties   map[string]Property `json:"properties"`
	Localization *Localization       `json:"localization,omitempty"`
}

type Property struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Value   any    `json:"value"`
	Meaning string `json:"meaning,omitempty"`
}

type Localization struct {
	URI     string   `json:"uri"`
	Default string   `json:"default"`
	Locales []string `json:"locales"`
}

func loadFont(dc *gg.Context, size float64) error {
	font, err := truetype.Parse(gobold.TTF)
	if err != nil {
		return fmt.Errorf("error parsing font: %v", err)
	}

	face := truetype.NewFace(font, &truetype.Options{
		Size: size,
	})
	dc.SetFontFace(face)
	return nil
}

func GenerateImage(tokenID string) error {
	// Load the Ferrari image
	imgFile, err := assets.Open("assets/ferrari.jpg")
	if err != nil {
		return fmt.Errorf("error opening ferrari image: %v", err)
	}
	defer imgFile.Close()

	baseImg, _, err := image.Decode(imgFile)
	if err != nil {
		return fmt.Errorf("error decoding ferrari image: %v", err)
	}

	// Create a new context with the same size as the Ferrari image
	bounds := baseImg.Bounds()
	dc := gg.NewContext(bounds.Max.X, bounds.Max.Y)

	// Draw the Ferrari image
	dc.DrawImage(baseImg, 0, 0)

	// Generate a unique color tint based on the tokenID
	hash := tokenID
	r := float64(int(hash[0])%255) / 255.0
	g := float64(int(hash[min(1, len(hash)-1)])%255) / 255.0
	b := float64(int(hash[min(2, len(hash)-1)])%255) / 255.0

	// Apply the unique color tint
	dc.SetRGBA(r, g, b, 0.2)
	dc.DrawRectangle(0, 0, float64(bounds.Max.X), float64(bounds.Max.Y))
	dc.Fill()

	// Add a subtle vignette effect
	vignette := gg.NewRadialGradient(
		float64(bounds.Max.X)/2, float64(bounds.Max.Y)/2, 0,
		float64(bounds.Max.X)/2, float64(bounds.Max.Y)/2, float64(bounds.Max.X),
	)
	vignette.AddColorStop(0, color.RGBA{0, 0, 0, 0})
	vignette.AddColorStop(1, color.RGBA{0, 0, 0, 40})
	dc.SetFillStyle(vignette)
	dc.DrawRectangle(0, 0, float64(bounds.Max.X), float64(bounds.Max.Y))
	dc.Fill()

	// Draw "Fleet Portfolio" text
	if err := loadFont(dc, float64(bounds.Max.Y)/12); err != nil {
		return fmt.Errorf("error loading title font: %v", err)
	}

	// Draw title with shadow
	title := "Fleet Portfolio"
	textWidth, textHeight := dc.MeasureString(title)
	x := (float64(bounds.Max.X) - textWidth) / 2
	y := textHeight + float64(bounds.Max.Y)/20

	// Draw shadow
	dc.SetRGBA(0, 0, 0, 0.5)
	dc.DrawString(title, x+2, y+2)

	// Draw main text
	dc.SetRGB(1, 1, 1)
	dc.DrawString(title, x, y)

	// Draw token ID below
	if err := loadFont(dc, float64(bounds.Max.Y)/15); err != nil {
		return fmt.Errorf("error loading ID font: %v", err)
	}

	// Format token ID for display
	formattedID := tokenID
	if len(tokenID) > 16 {
		formattedID = fmt.Sprintf("%s...%s", tokenID[:8], tokenID[len(tokenID)-8:])
	}

	idText := fmt.Sprintf("#%s", formattedID)
	textWidth, textHeight = dc.MeasureString(idText)
	x = (float64(bounds.Max.X) - textWidth) / 2
	y = float64(bounds.Max.Y) - textHeight - float64(bounds.Max.Y)/20

	// Draw ID shadow
	dc.SetRGBA(0, 0, 0, 0.5)
	dc.DrawString(idText, x+2, y+2)

	// Draw ID text
	dc.SetRGB(1, 1, 1)
	dc.DrawString(idText, x, y)

	// Save the image
	return dc.SavePNG(fmt.Sprintf("images/%s.png", tokenID))
}
