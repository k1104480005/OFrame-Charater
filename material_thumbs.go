package main

// Material preview thumbnails and deletion bindings: the identity page shows
// stored reference images as a visual grid (task 2.3 素材区) instead of text
// rows, so the user can tell materials apart and remove wrong ones.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/oframe/character-workbench/core/pipeline"
)

// MaterialThumbView is a downscaled PNG preview of one stored material image.
type MaterialThumbView struct {
	MaterialID string `json:"materialId"`
	PNG        string `json:"png"` // base64-encoded PNG thumbnail
}

// MaterialImageView is the full-resolution stored material image.
type MaterialImageView struct {
	MaterialID string `json:"materialId"`
	MIME       string `json:"mime"`
	Data       string `json:"data"` // base64-encoded original bytes
}

var materialMIMEByExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
}

// IdentityMaterialImage returns the stored material image at full resolution
// (capped at 20 MB) so the frontend can offer a zoomable preview.
func (a *App) IdentityMaterialImage(materialID string) (*MaterialImageView, error) {
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	mat, err := pkg.Material(materialID)
	if err != nil {
		return nil, err
	}
	abs, err := pkg.MaterialPath(mat)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("material image: read %q: %w", abs, err)
	}
	if len(data) > 20<<20 {
		return nil, fmt.Errorf("material image: %q is larger than 20 MB", abs)
	}
	mime := "application/octet-stream"
	if m, ok := materialMIMEByExt[filepath.Ext(strings.ToLower(abs))]; ok {
		mime = m
	}
	return &MaterialImageView{MaterialID: mat.ID, MIME: mime, Data: base64.StdEncoding.EncodeToString(data)}, nil
}

// IdentityRemoveMaterial deletes a material from the manifest and removes its
// stored file inside the package materials area (recoverable only by re-import).
func (a *App) IdentityRemoveMaterial(materialID string) error {
	pkg, err := a.requirePackage()
	if err != nil {
		return err
	}
	if err := pkg.RemoveMaterial(materialID); err != nil {
		return err
	}
	return a.identityChanged()
}

// IdentityMaterialThumbs returns PNG thumbnails for every stored material
// image. Materials that cannot be decoded are skipped (no preview cell).
func (a *App) IdentityMaterialThumbs() ([]MaterialThumbView, error) {
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	out := []MaterialThumbView{}
	for _, m := range pkg.Manifest().Materials {
		abs, err := pkg.MaterialPath(m)
		if err != nil {
			continue
		}
		pngData, err := materialThumbPNG(abs, 128)
		if err != nil {
			continue
		}
		out = append(out, MaterialThumbView{MaterialID: m.ID, PNG: base64.StdEncoding.EncodeToString(pngData)})
	}
	return out, nil
}

// materialThumbPNG decodes any supported image and nearest-neighbor downscales
// it to fit a square box (longest side = box), preserving aspect ratio. Pixel
// art previews stay crisp because sampling never interpolates.
func materialThumbPNG(path string, box int) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src, err := pipeline.DecodeImageAny(data)
	if err != nil {
		return nil, err
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("material thumb: empty image %q", path)
	}
	scale := 1.0
	if w > box || h > box {
		scale = math.Min(float64(box)/float64(w), float64(box)/float64(h))
	}
	tw := int(float64(w)*scale + 0.5)
	th := int(float64(h)*scale + 0.5)
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		sy := y * h / th
		for x := 0; x < tw; x++ {
			sx := x * w / tw
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
