// go-qrcode
// Copyright 2014 Tom Harwood

/*
Package qrcode implements a QR Code encoder.

A QR Code is a matrix (two-dimensional) barcode. Arbitrary content may be
encoded.

A QR Code contains error recovery information to aid reading damaged or
obscured codes. There are four levels of error recovery: qrcode.{Low, Medium,
High, Highest}. QR Codes with a higher recovery level are more robust to damage,
at the cost of being physically larger.

Three functions cover most use cases:

- Create a PNG image:

	var png []byte
	png, err := qrcode.Encode("https://example.org", qrcode.Medium, 256)

- Create a PNG image and write to a file:

	err := qrcode.WriteFile("https://example.org", qrcode.Medium, 256, "qr.png")

- Create a PNG image with custom colors and write to file:

	err := qrcode.WriteColorFile("https://example.org", qrcode.Medium, 256, color.Black, color.White, "qr.png")

All examples use the qrcode.Medium error Recovery Level and create a fixed
256x256px size QR Code. The last function creates a white on black instead of black
on white QR Code.

To generate a variable sized image instead, specify a negative size (in place of
the 256 above), such as -4 or -5. Larger negative numbers create larger images:
A size of -5 sets each module (QR Code "pixel") to be 5px wide/high.

- Create a PNG image (variable size, with minimum white padding) and write to a file:

	err := qrcode.WriteFile("https://example.org", qrcode.Medium, -5, "qr.png")

The maximum capacity of a QR Code varies according to the content encoded and
the error recovery level. The maximum capacity is 2,953 bytes, 4,296
alphanumeric characters, 7,089 numeric digits, or a combination of these.

This package implements a subset of QR Code 2005, as defined in ISO/IEC
18004:2006.
*/
package qrcode

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/nfnt/resize"
	"github.com/yougg/go-qrcode/bitset"
	"github.com/yougg/go-qrcode/reedsolomon"
)

const (
	otherPoint             = iota
	finderPatternPoint     = 1
	alignmentPatternsPoint = 2
	timingPatternsPoint    = 3
)

// Encode a QR Code and return a raw PNG image.
//
// size is both the image width and height in pixels. If size is too small then
// a larger image is silently returned. Negative values for size cause a
// variable sized image to be returned: See the documentation for Image().
//
// To serve over HTTP, remember to send a Content-Type: image/png header.
func Encode(content string, level RecoveryLevel, width, height, margin int) ([]byte, error) {
	var opts = []Option{
		Level(level),
		Width(width),
		Height(height),
		Margin(margin),
	}

	q, err := New(content, opts...)

	if err != nil {
		return nil, err
	}

	return q.PNG()
}

// WriteFile encodes, then writes a QR Code to the given filename in PNG format.
//
// size is both the image width and height in pixels. If size is too small then
// a larger image is silently written. Negative values for size cause a variable
// sized image to be written: See the documentation for Image().
func WriteFile(content string, level RecoveryLevel, size int, filename string, margin int) error {
	var opts = []Option{
		Level(level),
		Width(size),
		Height(size),
		Margin(margin),
	}

	q, err := New(content, opts...)

	if err != nil {
		return err
	}

	return q.WriteFile(filename)
}

// WriteColorFile encodes, then writes a QR Code to the given filename in PNG format.
// With WriteColorFile you can also specify the colors you want to use.
//
// size is both the image width and height in pixels. If size is too small then
// a larger image is silently written. Negative values for size cause a variable
// sized image to be written: See the documentation for Image().
func WriteColorFile(content string, level RecoveryLevel, size int, background, foreground color.Color, filename string, margin int) error {
	var opts = []Option{
		Level(level),
		Width(size),
		Height(size),
		Margin(margin),
		BackgroundColor(background),
		ForegroundColor(foreground),
	}

	q, err := New(content, opts...)
	if err != nil {
		return err
	}

	return q.WriteFile(filename)
}

func EncodeWithLogo(level RecoveryLevel, str string, logo image.Image, margin int) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	var colors color.Palette
	var opts = []Option{
		Level(level),
		Margin(margin),
	}
	code, err := New(str, opts...)
	if err != nil {
		return nil, err
	}

	logo = resize.Resize(40, 40, logo, resize.NearestNeighbor)
	for x := 0; x < logo.Bounds().Max.X; x++ {
		for y := 0; y < logo.Bounds().Max.Y; y++ {
			if contains(logo.At(x, y), colors) || len(colors) == 254 {
				continue
			}
			colors = append(colors, logo.At(x, y)) // FIXME colors to code.Image()
		}
	}
	img := code.Image()
	overlayLogo(img, logo)

	err = png.Encode(&buf, img)
	if err != nil {
		return nil, err
	}

	return &buf, nil
}

func contains(item color.Color, input color.Palette) bool {
	for _, v := range input {
		r1, g1, b1, a1 := item.RGBA()
		r2, g2, b2, a2 := v.RGBA()
		if r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2 {
			return true
		}
	}
	return false
}

func overlayLogo(dst, src image.Image) {
	offsetX := dst.Bounds().Max.X/2 - src.Bounds().Max.X/2
	offsetY := dst.Bounds().Max.Y/2 - src.Bounds().Max.Y/2

	for x := 0; x < src.Bounds().Max.X; x++ {
		for y := 0; y < src.Bounds().Max.Y; y++ {
			col := src.At(x, y)
			dst.(*image.Paletted).Set(x+offsetX, y+offsetY, col)
		}
	}
}

// A QRCode represents a valid encoded QRCode.
type QRCode struct {
	// Original content encoded.
	Content string

	// QR Code type.
	level         RecoveryLevel
	VersionNumber int

	// User settable drawing options.
	ForegroundColor color.Color
	BackgroundColor color.Color

	encoder *dataEncoder
	version qrCodeVersion

	data   *bitset.Bitset
	symbol *symbol
	mask   int

	width, height, margin int
	// set white space size.
	QuitZoneSize int
}

func (q *QRCode) Set(opts ...Option) {
	for _, opt := range opts {
		opt(q)
	}
	if nil == q.ForegroundColor {
		q.ForegroundColor = color.Black
	}
	if nil == q.BackgroundColor {
		q.BackgroundColor = color.White
	}
}

// New constructs a QRCode.
//
// 	var q *qrcode.QRCode
// 	q, err := qrcode.New("my content", qrcode.Medium)
//
// An error occurs if the content is too long.
func New(content string, opts ...Option) (*QRCode, error) {
	q := &QRCode{
		Content: content,
	}
	q.Set(opts...)

	encoders := []dataEncoderType{dataEncoderType1To9, dataEncoderType10To26, dataEncoderType27To40}

	var encoder *dataEncoder
	var encoded *bitset.Bitset
	var chosenVersion *qrCodeVersion
	var err error

	for _, t := range encoders {
		encoder = newDataEncoder(t)
		encoded, err = encoder.encode([]byte(content))

		if err != nil {
			continue
		}

		chosenVersion = chooseQRCodeVersion(q.level, encoder, encoded.Len())

		if chosenVersion != nil {
			break
		}
	}

	if err != nil {
		return nil, err
	} else if chosenVersion == nil {
		return nil, errors.New("content too long to encode")
	}

	q.VersionNumber = chosenVersion.version
	q.encoder = encoder
	q.data = encoded
	q.version = *chosenVersion
	// set quitZoneSize
	q.version.setQuietZoneSize(q.QuitZoneSize)
	q.encode(chosenVersion.numTerminatorBitsRequired(encoded.Len()))

	return q, nil
}

func newWithForcedVersion(content string, version int, level RecoveryLevel) (*QRCode, error) {
	var encoder *dataEncoder

	switch {
	case version >= 1 && version <= 9:
		encoder = newDataEncoder(dataEncoderType1To9)
	case version >= 10 && version <= 26:
		encoder = newDataEncoder(dataEncoderType10To26)
	case version >= 27 && version <= 40:
		encoder = newDataEncoder(dataEncoderType27To40)
	default:
		log.Fatalf("Invalid version %d (expected 1-40 inclusive)", version)
	}

	var encoded *bitset.Bitset
	encoded, err := encoder.encode([]byte(content))

	if err != nil {
		return nil, err
	}

	chosenVersion := getQRCodeVersion(level, version)

	if chosenVersion == nil {
		return nil, errors.New("cannot find QR Code version")
	}

	q := &QRCode{
		Content: content,

		level:         level,
		VersionNumber: chosenVersion.version,

		ForegroundColor: color.Black,
		BackgroundColor: color.White,

		encoder: encoder,
		data:    encoded,
		version: *chosenVersion,
	}

	q.encode(chosenVersion.numTerminatorBitsRequired(encoded.Len()))

	return q, nil
}

// Bitmap returns the QR Code as a 2D array of 1-bit pixels.
//
// bitmap[y][x] is true if the pixel at (x, y) is set.
//
// The bitmap includes the required "quiet zone" around the QR Code to aid
// decoding.
func (q *QRCode) Bitmap() [][]bool {
	return q.symbol.bitmap()
}

// Image returns the QR Code as an image.Image.
//
// A positive size sets a fixed image width and height (e.g. 256 yields an
// 256x256px image).
//
// Depending on the amount of data encoded, fixed size images can have different
// amounts of padding (white space around the QR Code). As an alternative, a
// variable sized image can be generated instead:
//
// A negative size causes a variable sized image to be returned. The image
// returned is the minimum size required for the QR Code. Choose a larger
// negative number to increase the scale of the image. e.g. a size of -5 causes
// each module (QR Code "pixel") to be 5px in size.
func (q *QRCode) Image() image.Image {
	// Minimum pixels (both width and height) required.
	realSize := q.symbol.size

	// Variable size support.
	if q.width < 0 {
		q.width = q.width * -1 * realSize
	}
	if q.height < 0 {
		q.height = q.height * -1 * realSize
	}

	// Actual pixels available to draw the symbol. Automatically increase the
	// image size if it's not large enough.
	if q.width < realSize {
		q.width = realSize
	}
	if q.height < realSize {
		q.height = realSize
	}

	// Size of each module drawn.
	pixelsPerModuleX := q.width / realSize
	pixelsPerModuleY := q.height / realSize

	// Center the symbol within the image.
	offsetX := (q.width - realSize*pixelsPerModuleX) / 2
	offsetY := (q.height - realSize*pixelsPerModuleY) / 2

	rect := image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{X: q.width, Y: q.height}}

	// Saves a few bytes to have them in this order
	p := color.Palette([]color.Color{q.BackgroundColor, q.ForegroundColor})
	img := image.NewPaletted(rect, p)

	for i := 0; i < q.width; i++ {
		for j := 0; j < q.height; j++ {
			img.Set(i, j, q.BackgroundColor)
		}
	}

	bitmap := q.symbol.bitmap()
	for y, row := range bitmap {
		for x, v := range row {
			if v {
				startX := x*pixelsPerModuleX + offsetX
				startY := y*pixelsPerModuleY + offsetY
				for i := startX; i < startX+pixelsPerModuleX; i++ {
					for j := startY; j < startY+pixelsPerModuleY; j++ {
						img.Set(i, j, q.ForegroundColor)
					}
				}
			}
		}
	}

	if float64(q.width)/float64(img.Bounds().Dx()) > 1 {
		tmp := scale(img, q.width)
		return &tmp
	}

	return img
}

// PNG returns the QR Code as a PNG image.
//
// size is both the image width and height in pixels. If size is too small then
// a larger image is silently returned. Negative values for size cause a
// variable sized image to be returned: See the documentation for Image().
func (q *QRCode) PNG() ([]byte, error) {
	img := q.Image()

	encoder := png.Encoder{CompressionLevel: png.BestCompression}

	var b bytes.Buffer
	err := encoder.Encode(&b, img)

	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

// Write writes the QR Code as a PNG image to io.Writer.
//
// size is both the image width and height in pixels. If size is too small then
// a larger image is silently written. Negative values for size cause a
// variable sized image to be written: See the documentation for Image().
func (q *QRCode) Write(out io.Writer) error {
	png, err := q.PNG()

	if err != nil {
		return err
	}
	_, err = out.Write(png)
	return err
}

// WriteFile writes the QR Code as a PNG image to the specified file.
//
// size is both the image width and height in pixels. If size is too small then
// a larger image is silently written. Negative values for size cause a
// variable sized image to be written: See the documentation for Image().
func (q *QRCode) WriteFile(filename string) error {
	png, err := q.PNG()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, png, os.FileMode(0644))
}

// encode completes the steps required to encode the QR Code. These include
// adding the terminator bits and padding, splitting the data into blocks and
// applying the error correction, and selecting the best data mask.
func (q *QRCode) encode(numTerminatorBits int) {
	q.addTerminatorBits(numTerminatorBits)
	q.addPadding()

	encoded := q.encodeBlocks()

	const numMasks int = 8
	penalty := 0

	for mask := 0; mask < numMasks; mask++ {
		var s *symbol
		var err error

		s, err = buildRegularSymbol(q.version, mask, encoded, q.margin)

		if err != nil {
			log.Panic(err.Error())
		}

		numEmptyModules := s.numEmptyModules()
		if numEmptyModules != 0 {
			log.Panicf("bug: numEmptyModules is %d (expected 0) (version=%d)", numEmptyModules, q.VersionNumber)
		}

		p := s.penaltyScore()

		// log.Printf("mask=%d p=%3d p1=%3d p2=%3d p3=%3d p4=%d\n", mask, p, s.penalty1(), s.penalty2(), s.penalty3(), s.penalty4())

		if q.symbol == nil || p < penalty {
			q.symbol = s
			q.mask = mask
			penalty = p
		}
	}
}

// addTerminatorBits adds final terminator bits to the encoded data.
//
// The number of terminator bits required is determined when the QR Code version
// is chosen (which itself depends on the length of the data encoded). The
// terminator bits are thus added after the QR Code version
// is chosen, rather than at the data encoding stage.
func (q *QRCode) addTerminatorBits(numTerminatorBits int) {
	q.data.AppendNumBools(numTerminatorBits, false)
}

// encodeBlocks takes the completed (terminated & padded) encoded data, splits
// the data into blocks (as specified by the QR Code version), applies error
// correction to each block, then interleaves the blocks together.
//
// The QR Code's final data sequence is returned.
func (q *QRCode) encodeBlocks() *bitset.Bitset {
	// Split into blocks.
	type dataBlock struct {
		data          *bitset.Bitset
		ecStartOffset int
	}

	block := make([]dataBlock, q.version.numBlocks())

	start := 0
	end := 0
	blockID := 0

	for _, b := range q.version.block {
		for j := 0; j < b.numBlocks; j++ {
			start = end
			end = start + b.numDataCodewords*8

			// Apply error correction to each block.
			numErrorCodewords := b.numCodewords - b.numDataCodewords
			block[blockID].data = reedsolomon.Encode(q.data.Substr(start, end), numErrorCodewords)
			block[blockID].ecStartOffset = end - start

			blockID++
		}
	}

	// Interleave the blocks.

	result := bitset.New()

	// Combine data blocks.
	working := true
	for i := 0; working; i += 8 {
		working = false

		for j, b := range block {
			if i >= block[j].ecStartOffset {
				continue
			}

			result.Append(b.data.Substr(i, i+8))

			working = true
		}
	}

	// Combine error correction blocks.
	working = true
	for i := 0; working; i += 8 {
		working = false

		for j, b := range block {
			offset := i + block[j].ecStartOffset
			if offset >= block[j].data.Len() {
				continue
			}

			result.Append(b.data.Substr(offset, offset+8))

			working = true
		}
	}

	// Append remainder bits.
	result.AppendNumBools(q.version.numRemainderBits, false)

	return result
}

// max returns the maximum of a and b.
func max(a int, b int) int {
	if a > b {
		return a
	}

	return b
}

// addPadding pads the encoded data upto the full length required.
func (q *QRCode) addPadding() {
	numDataBits := q.version.numDataBits()

	if q.data.Len() == numDataBits {
		return
	}

	// Pad to the nearest codeword boundary.
	q.data.AppendNumBools(q.version.numBitsToPadToCodeword(q.data.Len()), false)

	// Pad codewords 0b11101100 and 0b00010001.
	padding := [2]*bitset.Bitset{
		bitset.New(true, true, true, false, true, true, false, false),
		bitset.New(false, false, false, true, false, false, false, true),
	}

	// Insert pad codewords alternately.
	i := 0
	for numDataBits-q.data.Len() >= 8 {
		q.data.Append(padding[i])

		i = 1 - i // Alternate between 0 and 1.
	}

	if q.data.Len() != numDataBits {
		log.Panicf("BUG: got len %d, expected %d", q.data.Len(), numDataBits)
	}
}

// ToString produces a multi-line string that forms a QR-code image.
func (q *QRCode) ToString(inverseColor bool) string {
	bits := q.Bitmap()
	var buf bytes.Buffer
	for y := range bits {
		for x := range bits[y] {
			if bits[y][x] != inverseColor {
				buf.WriteString("  ")
			} else {
				buf.WriteString("██")
			}
		}
		buf.WriteString("\n")
	}
	return buf.String()
}

// getPointType return point type.
func (q *QRCode) getPointType(x, y int) int {
	qrSize := q.version.symbolSize()
	// finderPatternPoint
	if 0 <= x-q.symbol.quietZoneSize && x-q.symbol.quietZoneSize <= finderPatternSize && 0 <= y-q.symbol.quietZoneSize && y-q.symbol.quietZoneSize <= finderPatternSize { // top left
		return finderPatternPoint
	}
	if qrSize-finderPatternSize <= x-q.symbol.quietZoneSize && x-q.symbol.quietZoneSize <= qrSize && 0 <= y-q.symbol.quietZoneSize && y-q.symbol.quietZoneSize <= finderPatternSize { // top right
		return finderPatternPoint
	}
	if 0 <= x-q.symbol.quietZoneSize && x-q.symbol.quietZoneSize <= finderPatternSize && qrSize-finderPatternSize <= y-q.symbol.quietZoneSize && y-q.symbol.quietZoneSize <= qrSize { // bottom left
		return finderPatternPoint
	}
	// alignmentPatternsPoint
	alignmentPatternSize := len(alignmentPattern)
	for _, x0 := range alignmentPatternCenter[q.version.version] {
	TMP:
		for _, y0 := range alignmentPatternCenter[q.version.version] {
			// m.symbol.set2dPattern(x-2, y-2, alignmentPattern)
			if x0-2 <= x-q.symbol.quietZoneSize && x-q.symbol.quietZoneSize < x0-2+alignmentPatternSize && y0-2 <= y-q.symbol.quietZoneSize && y-q.symbol.quietZoneSize < y0-2+alignmentPatternSize {
				// there is alignment patterns.
				for j, row := range alignmentPattern {
					for i, value := range row {
						if value != q.symbol.get(x0-2+i+q.symbol.quietZoneSize, y0-2+j+q.symbol.quietZoneSize) {
							continue TMP
						}
					}
				}
				return alignmentPatternsPoint
			}
		}
	}
	// timingPatternsPoint
	if (finderPatternSize+1 <= x && x <= q.symbol.size-finderPatternSize && y == finderPatternSize-1) || (x == finderPatternSize-1 && finderPatternSize+1 <= y && y <= q.symbol.size-finderPatternSize) {
		return timingPatternsPoint
	}
	return 0
}
