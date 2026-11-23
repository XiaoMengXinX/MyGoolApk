package app

import (
	"bufio"
	"encoding/binary"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
)

// jpegOrientation reads the EXIF orientation of a JPEG, 1 when it names
// none. Android's decoder draws a picture as stored, so a photo the camera
// kept on its side shows on its side unless the file is turned first; the
// CDN's sized copies are turned already, the originals are not.
func jpegOrientation(r io.Reader) int {
	br := bufio.NewReader(r)
	var b [4]byte
	if _, err := io.ReadFull(br, b[:2]); err != nil || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	for {
		if _, err := io.ReadFull(br, b[:4]); err != nil || b[0] != 0xFF {
			return 1
		}
		marker, size := b[1], int(binary.BigEndian.Uint16(b[2:]))-2
		if marker == 0xDA || size < 0 { // the image data begins: no EXIF before it
			return 1
		}
		seg := make([]byte, size)
		if _, err := io.ReadFull(br, seg); err != nil {
			return 1
		}
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
	}
}

// tiffOrientation reads tag 0x0112 of the first IFD of a TIFF header.
func tiffOrientation(t []byte) int {
	var order binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int(order.Uint32(t[4:8]))
	if ifd+2 > len(t) {
		return 1
	}
	n := int(order.Uint16(t[ifd:]))
	for i := range n {
		e := ifd + 2 + i*12
		if e+12 > len(t) {
			return 1
		}
		if order.Uint16(t[e:]) == 0x0112 {
			if o := int(order.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}

// orientJPEG turns the JPEG at path upright by its EXIF orientation, writing
// it again without the tag. A file that is upright already, or is no JPEG,
// stays as it is.
func orientJPEG(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	o := jpegOrientation(f)
	if o == 1 {
		f.Close()
		return nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return err
	}
	src, err := jpeg.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	out := orient(src, o)
	tmp, err := os.CreateTemp(filepath.Dir(path), "orient-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := jpeg.Encode(tmp, out, &jpeg.Options{Quality: 92}); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// orient applies EXIF orientation o (2 to 8) to src.
func orient(src image.Image, o int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
	ow, oh := w, h
	if o >= 5 {
		ow, oh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			si, di := rgba.PixOffset(x, y), dst.PixOffset(dx, dy)
			copy(dst.Pix[di:di+4], rgba.Pix[si:si+4])
		}
	}
	return dst
}
