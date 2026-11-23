package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// exifJPEG is a JPEG of img with an EXIF orientation tag of o.
func exifJPEG(t *testing.T, img image.Image, o uint16) []byte {
	var body bytes.Buffer
	if err := jpeg.Encode(&body, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	var tiff bytes.Buffer
	tiff.WriteString("MM")
	binary.Write(&tiff, binary.BigEndian, uint16(42))
	binary.Write(&tiff, binary.BigEndian, uint32(8))
	binary.Write(&tiff, binary.BigEndian, uint16(1))
	binary.Write(&tiff, binary.BigEndian, []uint16{0x0112, 3})
	binary.Write(&tiff, binary.BigEndian, uint32(1))
	binary.Write(&tiff, binary.BigEndian, []uint16{o, 0})
	binary.Write(&tiff, binary.BigEndian, uint32(0))
	app1 := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	var out bytes.Buffer
	out.Write([]byte{0xFF, 0xD8, 0xFF, 0xE1})
	binary.Write(&out, binary.BigEndian, uint16(len(app1)+2))
	out.Write(app1)
	out.Write(body.Bytes()[2:])
	return out.Bytes()
}

// A photo stored on its side comes out of the store upright.
func TestOrientJPEGTurnsSidewaysPhotosUpright(t *testing.T) {
	// 4 wide and 2 tall, red on the left half: orientation 6 is a quarter
	// turn clockwise, which puts the red on top.
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			c := color.RGBA{0, 0, 255, 255}
			if x < 20 {
				c = color.RGBA{255, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	path := filepath.Join(t.TempDir(), "p.jpg")
	if err := os.WriteFile(path, exifJPEG(t, img, 6), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := orientJPEG(path); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(path)
	defer f.Close()
	out, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := out.Bounds(); b.Dx() != 20 || b.Dy() != 40 {
		t.Fatalf("size %v, want 20x40", b)
	}
	if r, _, bl, _ := out.At(10, 5).RGBA(); r < bl {
		t.Errorf("the top is not red")
	}
	f.Seek(0, 0)
	if o := jpegOrientation(f); o != 1 {
		t.Errorf("orientation %d after turning, want 1", o)
	}
}
