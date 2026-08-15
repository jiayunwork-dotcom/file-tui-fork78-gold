package preview

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

type PreviewType int

const (
	PreviewText PreviewType = iota
	PreviewBinary
	PreviewImage
	PreviewDirectory
)

type Preview struct {
	Content   []string
	Scroll    int
	Width     int
	Height    int
	FilePath  string
	Type      PreviewType
	IsOpen    bool
	ThemeName string
}

func NewPreview() *Preview {
	return &Preview{
		Content:   []string{},
		Scroll:    0,
		IsOpen:    false,
		ThemeName: "dark",
	}
}

func (p *Preview) Load(path string, width, height int, themeName string) error {
	p.FilePath = path
	p.Width = width
	p.Height = height
	p.Scroll = 0
	p.ThemeName = themeName

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		p.Type = PreviewDirectory
		p.loadDirectory(path)
		return nil
	}

	ext := strings.ToLower(filepath.Ext(path))
	if isImageExt(ext) {
		if p.loadImage(path) {
			return nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if isBinary(data) {
		p.Type = PreviewBinary
		p.loadBinary(data)
	} else {
		p.Type = PreviewText
		p.loadText(path)
	}

	return nil
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	}
	return false
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	checkLen := 8000
	if len(data) < checkLen {
		checkLen = len(data)
	}

	zeros := 0
	for i := 0; i < checkLen; i++ {
		b := data[i]
		if b == 0 {
			zeros++
		}
		if b < 7 && b != 9 && b != 10 && b != 13 {
			return true
		}
	}

	if float64(zeros)/float64(checkLen) > 0.3 {
		return true
	}
	return false
}

func (p *Preview) loadText(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		p.Content = []string{"Error: " + err.Error()}
		return
	}

	lexer := lexers.Match(path)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	var styleName string
	if p.ThemeName == "light" {
		styleName = "github"
	} else {
		styleName = "monokai"
	}

	style := styles.Get(styleName)
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, string(data))
	if err != nil {
		p.Content = p.loadPlainText(data)
		return
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		p.Content = p.loadPlainText(data)
		return
	}

	lines := strings.Split(buf.String(), "\n")
	for i, line := range lines {
		if len(line) > p.Width-4 {
			line = line[:p.Width-4]
		}
		lines[i] = line
	}

	if len(lines) > 500 {
		lines = lines[:500]
		lines = append(lines, "... (truncated)")
	}

	p.Content = lines
}

func (p *Preview) loadPlainText(data []byte) []string {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if len(line) > p.Width-4 {
			line = line[:p.Width-4]
		}
		lines[i] = line
	}

	if len(lines) > 500 {
		lines = lines[:500]
		lines = append(lines, "... (truncated)")
	}

	return lines
}

func (p *Preview) loadBinary(data []byte) {
	lines := []string{}
	maxBytes := 10000
	if len(data) < maxBytes {
		maxBytes = len(data)
	}

	for i := 0; i < maxBytes; i += 16 {
		end := i + 16
		if end > maxBytes {
			end = maxBytes
		}

		chunk := data[i:end]
		hexStr := hex.EncodeToString(chunk)

		hexFmt := make([]string, 0, 16)
		for j := 0; j < len(chunk); j += 2 {
			if j+1 < len(chunk) {
				hexFmt = append(hexFmt, hexStr[j:j+2]+hexStr[j+1:j+2])
			} else {
				hexFmt = append(hexFmt, hexStr[j:]+"  ")
			}
		}

		for len(hexFmt) < 8 {
			hexFmt = append(hexFmt, "    ")
		}

		ascii := make([]byte, len(chunk))
		for j, b := range chunk {
			if b >= 32 && b <= 126 {
				ascii[j] = b
			} else {
				ascii[j] = '.'
			}
		}

		hexPart := strings.Join(hexFmt, " ")
		line := fmt.Sprintf("%08x  %-39s |%s|", i, hexPart, string(ascii))
		lines = append(lines, line)
	}

	if maxBytes < len(data) {
		lines = append(lines, "... (truncated, total "+fmt.Sprintf("%d", len(data))+" bytes)")
	}

	p.Content = lines
}

func (p *Preview) loadDirectory(path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		p.Content = []string{"Error: " + err.Error()}
		return
	}

	lines := []string{
		fmt.Sprintf("Directory: %s", path),
		fmt.Sprintf("Total items: %d", len(entries)),
		"",
		"Contents:",
	}

	for i, e := range entries {
		if i >= 100 {
			lines = append(lines, "... (more items)")
			break
		}
		name := e.Name()
		prefix := "  "
		if e.IsDir() {
			prefix = "d "
		}
		lines = append(lines, prefix+name)
	}

	p.Content = lines
}

func (p *Preview) loadImage(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	var img image.Image
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".png":
		img, err = png.Decode(file)
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(file)
	case ".gif":
		img, err = gif.Decode(file)
	default:
		return false
	}

	if err != nil {
		return false
	}

	p.Type = PreviewImage
	p.Content = imageToASCII(img, p.Width-4)
	return true
}

func imageToASCII(img image.Image, maxWidth int) []string {
	bounds := img.Bounds()
	imgWidth := bounds.Dx()
	imgHeight := bounds.Dy()

	ratio := 2.0
	scale := float64(maxWidth) / float64(imgWidth)
	if scale*float64(imgHeight)/ratio > 30 {
		scale = 30.0 * ratio / float64(imgHeight)
	}

	newWidth := int(float64(imgWidth) * scale)
	newHeight := int(float64(imgHeight) / ratio * scale)

	lines := make([]string, newHeight)
	chars := " .'`^,:;Il!i~+_-?][}{1)(|/tfjrxnuvczXYUJCLQ0OZmwqpdbkhao*#MW&8%B@$"

	for y := 0; y < newHeight; y++ {
		line := ""
		for x := 0; x < newWidth; x++ {
			srcX := int(float64(x) / scale)
			srcY := int(float64(y) * ratio / scale)

			if srcX >= imgWidth {
				srcX = imgWidth - 1
			}
			if srcY >= imgHeight {
				srcY = imgHeight - 1
			}

			pixel := img.At(srcX+bounds.Min.X, srcY+bounds.Min.Y)
			r, g, b, _ := pixel.RGBA()

			brightness := float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(b>>8)*0.114
			idx := int(brightness / 255.0 * float64(len(chars)-1))
			if idx < 0 {
				idx = 0
			}
			if idx >= len(chars) {
				idx = len(chars) - 1
			}

			line += colorizeChar(chars[idx], r, g, b)
		}
		lines[y] = line
	}

	return lines
}

func colorizeChar(c byte, r, g, b uint32) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)))
	return style.Render(string(c))
}

func (p *Preview) ScrollUp() {
	if p.Scroll > 0 {
		p.Scroll--
	}
}

func (p *Preview) ScrollDown() {
	visible := p.Height - 4
	if p.Scroll+visible < len(p.Content) {
		p.Scroll++
	}
}

func (p *Preview) PageUp() {
	visible := p.Height - 4
	p.Scroll = max(0, p.Scroll-visible)
}

func (p *Preview) PageDown() {
	visible := p.Height - 4
	p.Scroll = min(p.Scroll+visible, max(0, len(p.Content)-visible))
}

func (p *Preview) View() string {
	if !p.IsOpen || len(p.Content) == 0 {
		return ""
	}

	visible := p.Height - 4
	if visible <= 0 {
		return ""
	}

	end := p.Scroll + visible
	if end > len(p.Content) {
		end = len(p.Content)
	}

	content := p.Content[p.Scroll:end]

	for len(content) < visible {
		content = append(content, "")
	}

	header := fmt.Sprintf(" Preview: %s ", p.FilePath)
	if len(header) > p.Width-2 {
		header = header[:p.Width-2]
	}

	for i, line := range content {
		if len(line) > p.Width-4 {
			content[i] = line[:p.Width-4]
		}
	}

	body := strings.Join(content, "\n")
	footer := fmt.Sprintf(" %d/%d lines (↑/↓ scroll, Esc close) ", p.Scroll+1, len(p.Content))

	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Width(p.Width).Background(lipgloss.Color("#336699")).Bold(true).Render(header),
		lipgloss.NewStyle().Width(p.Width).Height(visible).Background(lipgloss.Color("#1a1a1a")).Render(body),
		lipgloss.NewStyle().Width(p.Width).Background(lipgloss.Color("#333333")).Render(footer),
	)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
