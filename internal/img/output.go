// Copyright © 2020 The Homeport Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package img

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"math"
	"path/filepath" // CHANGE
	"strings"

	"github.com/esimov/stackblur-go"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/gonvenience/bunt"
	"github.com/gonvenience/term"
	imgfont "golang.org/x/image/font"
)

const (
	red    = "#ED655A"
	yellow = "#E1C04C"
	green  = "#71BD47"
)

type Scaffold struct {
	content bunt.String

	factor float64

	columns int

	defaultForegroundColor color.Color

	clipCanvas bool

	drawDecorations bool
	drawShadow      bool

	shadowBaseColor string
	shadowRadius    uint8
	shadowOffsetX   float64
	shadowOffsetY   float64

	padding float64
	margin  float64

	regular     imgfont.Face
	bold        imgfont.Face
	italic      imgfont.Face
	boldItalic  imgfont.Face
	lineSpacing float64
	tabSpaces   int
}

// CHANGE
func NewImageCreator() Scaffold {
	f := config.Factor

	// Load fonts from embedded filesystem
	regular, bold, italic, boldItalic, err := LoadFontsFromEmbedded(
		fontsFS,
		config.FontDir,
		f*config.FontSize,
		config.FontDPI,
	)
	if err != nil {
		panic("failed to load embedded fonts from " + config.FontDir + ": " + err.Error())
	}

	return Scaffold{
		defaultForegroundColor: bunt.LightGray,

		factor: f,

		margin:  f * config.Margin,
		padding: f * config.Padding,

		drawDecorations: config.DrawDecorations,
		drawShadow:      config.DrawShadow,

		shadowBaseColor: config.ShadowBaseColor,
		shadowRadius:    uint8(math.Min(f*config.ShadowRadius, 255)),
		shadowOffsetX:   f * config.ShadowOffsetX,
		shadowOffsetY:   f * config.ShadowOffsetY,

		regular:     regular,
		bold:        bold,
		italic:      italic,
		boldItalic:  boldItalic,
		lineSpacing: config.LineSpacing,
		tabSpaces:   config.TabSpaces,
	}
}

func (s *Scaffold) SetFontFaceRegular(face imgfont.Face) { s.regular = face }

func (s *Scaffold) SetFontFaceBold(face imgfont.Face) { s.bold = face }

func (s *Scaffold) SetFontFaceItalic(face imgfont.Face) { s.italic = face }

func (s *Scaffold) SetFontFaceBoldItalic(face imgfont.Face) { s.boldItalic = face }

func (s *Scaffold) SetColumns(columns int) { s.columns = columns }

func (s *Scaffold) DrawDecorations(value bool) { s.drawDecorations = value }

func (s *Scaffold) DrawShadow(value bool) { s.drawShadow = value }

func (s *Scaffold) ClipCanvas(value bool) { s.clipCanvas = value }

func (s *Scaffold) GetFixedColumns() int {
	if s.columns != 0 {
		return s.columns
	}

	columns, _ := term.GetTerminalSize()
	return columns
}

// CHANGE
func (s *Scaffold) AddCommand(args ...string) error {
	// Build command string with ANSI color codes from config
	promptColor := hexToANSI(config.PromptColor)
	commandColor := hexToANSI(config.CommandColor)
	reset := "\x1b[0m"

	formatted := fmt.Sprintf("%s%s%s %s%s%s\n",
		promptColor, config.Prompt, reset,
		commandColor, strings.Join(args, " "), reset,
	)

	return s.AddContent(strings.NewReader(formatted))
}

func (s *Scaffold) AddContent(in io.Reader) error {
	parsed, err := bunt.ParseStream(in)
	if err != nil {
		return fmt.Errorf("failed to parse input stream: %w", err)
	}

	var tmp bunt.String
	var counter int
	for _, cr := range *parsed {
		counter++

		if cr.Symbol == '\n' {
			counter = 0
		}

		// Add an additional newline in case the column
		// count is reached and line wrapping is needed
		if counter > s.GetFixedColumns() {
			counter = 0
			tmp = append(tmp, bunt.ColoredRune{
				Settings: cr.Settings,
				Symbol:   '\n',
			})
		}

		tmp = append(tmp, cr)
	}

	s.content = append(s.content, tmp...)

	return nil
}

func (s *Scaffold) fontHeight() float64 {
	return float64(s.regular.Metrics().Height >> 6)
}

func (s *Scaffold) measureContent() (width float64, height float64) {
	var tmp = make([]rune, len(s.content))
	for i, cr := range s.content {
		tmp[i] = cr.Symbol
	}

	lines := strings.Split(
		strings.TrimSuffix(
			string(tmp),
			"\n",
		),
		"\n",
	)

	// temporary drawer for reference calucation
	tmpDrawer := &imgfont.Drawer{Face: s.regular}

	// width, either by using longest line, or by fixed column value
	switch s.columns {
	case 0: // unlimited: max width of all lines
		for _, line := range lines {
			advance := tmpDrawer.MeasureString(line)
			if lineWidth := float64(advance >> 6); lineWidth > width {
				width = lineWidth
			}
		}

	default: // fixed: max width based on column count
		width = float64(tmpDrawer.MeasureString(strings.Repeat("a", s.GetFixedColumns())) >> 6)
	}

	// height, lines times font height and line spacing
	height = s.fontHeight() * (float64(len(lines)-1)*s.lineSpacing + 1) // CHANGE

	return width, height
}

func (s *Scaffold) image() (image.Image, error) {
	var f = func(value float64) float64 { return s.factor * value }

	var (
		corner   = f(6)
		radius   = f(9)
		distance = f(25)
	)

	contentWidth, contentHeight := s.measureContent()

	// Make sure the output window is big enough in case no content or very few
	// content will be rendered
	contentWidth = math.Max(contentWidth, 3*distance+3*radius)

	marginX, marginY := s.margin, s.margin
	paddingX, paddingY := s.padding, s.padding

	xOffset := marginX
	yOffset := marginY

	var titleOffset float64
	if s.drawDecorations {
		titleOffset = f(40)
	}

	width := contentWidth + 2*marginX + 2*paddingX
	height := contentHeight + 2*marginY + 2*paddingY + titleOffset

	dc := gg.NewContext(int(width), int(height))

	// Optional: Apply blurred rounded rectangle to mimic the window shadow
	//
	if s.drawShadow {
		xOffset -= s.shadowOffsetX / 2
		yOffset -= s.shadowOffsetY / 2

		bc := gg.NewContext(int(width), int(height))
		bc.DrawRoundedRectangle(xOffset+s.shadowOffsetX, yOffset+s.shadowOffsetY, width-2*marginX, height-2*marginY, corner)
		bc.SetHexColor(s.shadowBaseColor)
		bc.Fill()

		src := bc.Image()
		dst := image.NewNRGBA(src.Bounds())
		if err := stackblur.Process(dst, src, uint32(s.shadowRadius)); err != nil {
			return nil, err
		}

		dc.DrawImage(dst, 0, 0)
	}

	// CHANGE
	// Draw rounded rectangle with outline to produce impression of a window
	//
	dc.DrawRoundedRectangle(xOffset, yOffset, width-2*marginX, height-2*marginY, corner)
	dc.SetHexColor(config.BackgroundColor)
	dc.Fill()

	dc.DrawRoundedRectangle(xOffset, yOffset, width-2*marginX, height-2*marginY, corner)
	dc.SetHexColor(config.OutlineColor)
	dc.SetLineWidth(f(1))
	dc.Stroke()

	// Optional: Draw window decorations (i.e. three buttons) to produce the
	// impression of an actional window
	//
	if s.drawDecorations {
		for i, color := range []string{red, yellow, green} {
			dc.DrawCircle(xOffset+paddingX+float64(i)*distance+f(4), yOffset+paddingY+f(4), radius)
			dc.SetHexColor(color)
			dc.Fill()
		}
	}

	// Apply the actual text into the prepared content area of the window
	//
	var x, y = xOffset + paddingX, yOffset + paddingY + titleOffset + float64(s.regular.Metrics().Ascent>>6)
	for _, cr := range s.content {
		switch cr.Settings & 0x1C {
		case 4:
			dc.SetFontFace(s.bold)

		case 8:
			dc.SetFontFace(s.italic)

		case 12:
			dc.SetFontFace(s.boldItalic)

		default:
			dc.SetFontFace(s.regular)
		}

		str := string(cr.Symbol)
		w, h := dc.MeasureString(str)

		// background color
		switch cr.Settings & 0x02 { //nolint:gocritic
		case 2:
			dc.SetRGB255(
				int((cr.Settings>>32)&0xFF), // #nosec G115
				int((cr.Settings>>40)&0xFF), // #nosec G115
				int((cr.Settings>>48)&0xFF), // #nosec G115
			)

			dc.DrawRectangle(x, y-h+12, w, h)
			dc.Fill()
		}

		// foreground color
		switch cr.Settings & 0x01 {
		case 1:
			dc.SetRGB255(
				int((cr.Settings>>8)&0xFF),  // #nosec G115
				int((cr.Settings>>16)&0xFF), // #nosec G115
				int((cr.Settings>>24)&0xFF), // #nosec G115
			)

		default:
			dc.SetColor(s.defaultForegroundColor)
		}

		switch str {
		case "\n":
			x = xOffset + paddingX
			y += h * s.lineSpacing
			continue

		case "\t":
			x += w * float64(s.tabSpaces)
			continue

		case "✗", "ˣ": // mitigate issue #1 by replacing it with a similar character
			str = "×"
		}

		dc.DrawString(str, x, y)

		// There seems to be no font face based way to do an underlined
		// string, therefore manually draw a line under each character
		if cr.Settings&0x1C == 16 {
			dc.DrawLine(x, y+f(4), x+w, y+f(4))
			dc.SetLineWidth(f(1))
			dc.Stroke()
		}

		x += w
	}

	return dc.Image(), nil
}

// Write writes the scaffold content as PNG into the provided writer
//
// Deprecated: Use [Scaffold.WritePNG] instead.
func (s *Scaffold) Write(w io.Writer) error {
	return s.WritePNG(w)
}

// WritePNG writes the scaffold content as PNG into the provided writer
func (s *Scaffold) WritePNG(w io.Writer) error {
	img, err := s.image()
	if err != nil {
		return err
	}

	// Optional: Clip image to minimum size by removing all surrounding transparent pixels
	//
	if s.clipCanvas {
		if imgRGBA, ok := img.(*image.RGBA); ok {
			var minX, minY = math.MaxInt, math.MaxInt
			var maxX, maxY = 0, 0

			var bounds = imgRGBA.Bounds()
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					r, g, b, a := imgRGBA.At(x, y).RGBA()
					isTransparent := r == 0 && g == 0 && b == 0 && a == 0

					if !isTransparent {
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
					}
				}
			}

			img = imgRGBA.SubImage(image.Rect(minX, minY, maxX, maxY))
		}
	}

	return png.Encode(w, img)
}

// WriteRaw writes the scaffold content as-is into the provided writer
func (s *Scaffold) WriteRaw(w io.Writer) error {
	_, err := w.Write([]byte(s.content.String()))
	return err
}

// CHANGE
func LoadFontsFromEmbedded(fsys fs.FS, dir string, fontSize float64, fontDPI float64) (
	regular, bold, italic, boldItalic imgfont.Face,
	err error,
) {
	// Read directory contents from embedded FS
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to read embedded font directory: %w", err)
	}

	// Find font files matching each required pattern
	styles := []string{"Regular", "Bold", "Italic", "BoldItalic"}
	foundFiles := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ttf") {
			continue
		}

		for _, style := range styles {
			suffix := fmt.Sprintf("-%s.ttf", style)
			if strings.HasSuffix(entry.Name(), suffix) {
				if existing, exists := foundFiles[style]; exists {
					return nil, nil, nil, nil, fmt.Errorf(
						"multiple files found for %s style: %s and %s",
						style, existing, entry.Name(),
					)
				}
				foundFiles[style] = entry.Name()
			}
		}
	}

	// Verify all required fonts were found
	for _, style := range styles {
		if _, found := foundFiles[style]; !found {
			return nil, nil, nil, nil, fmt.Errorf(
				"missing required font file: no file matching *-%s.ttf found in %s",
				style, dir,
			)
		}
	}

	// Load all fonts
	fontFaceOptions := &truetype.Options{Size: fontSize, DPI: fontDPI}
	faces := make(map[string]imgfont.Face)

	for _, style := range styles {
		fontPath := filepath.Join(dir, foundFiles[style])
		face, err := loadFontFileFromFS(fsys, fontPath, fontFaceOptions)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to load %s font: %w", style, err)
		}
		faces[style] = face
	}

	return faces["Regular"], faces["Bold"], faces["Italic"], faces["BoldItalic"], nil
}

// loadFontFileFromFS reads and parses a TrueType font file from an embedded filesystem
func loadFontFileFromFS(fsys fs.FS, path string, options *truetype.Options) (imgfont.Face, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}

	font, err := truetype.Parse(data)
	if err != nil {
		return nil, err
	}

	return truetype.NewFace(font, options), nil
}

// GetFontOptions returns the font size and DPI used by this scaffold.
// This is useful for loading custom fonts with the same dimensions.
func (s *Scaffold) GetFontOptions() (fontSize float64, fontDPI float64) {
	return s.factor * config.FontSize, config.FontDPI
}

// hexToRGB converts a hex color string to RGB values
func hexToRGB(hex string) (r, g, b int) {
	hex = strings.TrimPrefix(hex, "#")
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return
}

// hexToANSI converts a hex color to an ANSI foreground color escape code
func hexToANSI(hexColor string) string {
	r, g, b := hexToRGB(hexColor)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}
