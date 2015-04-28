package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	draw "golang.org/x/image/draw"
)

func MustAtoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

var IMAGE_URL = flag.String("image-url", os.Getenv("IMAGE_URL"),
	"Base URL for image files to be pulled")
var WATERMARK_SIZE = flag.Int("watermark-size", MustAtoi(os.Getenv("WATERMARK_SIZE")),
	"Size of the watermark to apply")

//go:generate go-bindata watermark.png
var WATERMARK draw.Image

func InitWatermark(size int) draw.Image {
	wbin, err := Asset("watermark.png")
	if err != nil {
		panic(err)
	}

	wmfull, err := png.Decode(bytes.NewReader(wbin))
	if err != nil {
		panic(err)
	}

	b := image.Rect(0, 0, size, size)
	watermark := image.NewRGBA(b)
	draw.CatmullRom.Scale(watermark, b, wmfull, wmfull.Bounds(), nil)
	return watermark
}

func doWatermark(imgtype string, imgbody io.Reader, output io.Writer) error {

	var img image.Image
	var err error

	imgtype = strings.ToLower(imgtype)
	switch imgtype {
	case "jpeg", "jpg":
		img, err = jpeg.Decode(imgbody)
	case "png":
		img, err = png.Decode(imgbody)
	case "gif":
		img, err = gif.Decode(imgbody)
	default:
		err = fmt.Errorf("unsupported image format %s", imgtype)
	}

	if err != nil {
		return err
	}

	b := img.Bounds()
	offset := b.Max.Sub(WATERMARK.Bounds().Max)
	m := image.NewRGBA(b)
	draw.Draw(m, b, img, image.ZP, draw.Over)
	draw.Draw(m, WATERMARK.Bounds().Add(offset), WATERMARK, image.ZP, draw.Over)

	switch imgtype {
	case "jpeg", "jpg":
		jpeg.Encode(output, m, nil)
	case "png":
		png.Encode(output, m)
	case "gif":
		gif.Encode(output, m, nil)
	}
	return nil
}

func proxyImage(w http.ResponseWriter, r *http.Request) {

	full_url := *IMAGE_URL
	if full_url[len(full_url)-1] != '/' {
		full_url += "/"
	}
	full_url += r.URL.Path[11:]
	rimg, err := http.Get(full_url)
	if err != nil {
		log.Println("ERROR: HTTP error", err)
		http.Error(w, "internal error", 500)
		return
	}
	defer rimg.Body.Close()

	if rimg.StatusCode >= 400 && rimg.StatusCode < 500 {
		log.Printf("ERROR: HTTP status code %d requesting %s", rimg.StatusCode, full_url)
		http.Error(w, "error requesting resource", 400)
		return
	} else if rimg.StatusCode >= 500 {
		log.Printf("ERROR: HTTP status code %d requesting %s", rimg.StatusCode, full_url)
		http.Error(w, "internal error", 500)
		return
	}

	ct := rimg.Header.Get("Content-Type")
	if ct == "" {
		log.Printf("ERROR: no content-type on %s", full_url)
		http.Error(w, "requested resource is not a media file", 400)
		return
	}

	mimetype, _, err := mime.ParseMediaType(ct)
	if err != nil || mimetype[:6] != "image/" {
		log.Printf("ERROR: invalid content type on %s", full_url)
		http.Error(w, "invalid content type", 400)
		return
	}

	err = doWatermark(mimetype[6:], rimg.Body, w)
	if err != nil {
		log.Printf("ERROR: applying watermark: %s", err)
		http.Error(w, "error applying watermark", 500)
		return
	}
}

func main() {
	flag.Parse()
	flag.VisitAll(func(f *flag.Flag) {
		log.Printf("%v: %v\n", f.Name, f.Value)
	})

	WATERMARK = InitWatermark(*WATERMARK_SIZE)
	http.HandleFunc("/watermark/", proxyImage)
	http.ListenAndServe(":8080", nil)
}
