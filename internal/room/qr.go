package room

import (
	"bytes"
	"fmt"

	"github.com/skip2/go-qrcode"
)

// GenerateSVG encodes the given content into a scalable vector graphics (SVG) QR code.
func GenerateSVG(content string, size int) ([]byte, error) {
	if size <= 0 {
		size = 280
	}
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("create qr code: %w", err)
	}

	bitmap := qr.Bitmap()
	modules := len(bitmap)
	if modules == 0 {
		return nil, fmt.Errorf("empty qr bitmap")
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" shape-rendering="crispEdges">`, modules, modules, size, size)
	buf.WriteString(`<rect width="100%" height="100%" fill="#ffffff"/>`)
	buf.WriteString(`<path fill="#000000" d="`)
	for y, row := range bitmap {
		for x, val := range row {
			if val {
				fmt.Fprintf(&buf, "M%d %dh1v1h-1z", x, y)
			}
		}
	}
	buf.WriteString(`"/></svg>`)

	return buf.Bytes(), nil
}
