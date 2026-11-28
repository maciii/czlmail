package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/woodchen-ink/czlmail/desktop/internal/imgcache"
)

// 正文图片按文件名猜成图片请求 (原本标成 application/octet-stream): 返回的类型以内容为准, 不是图片就不返回。
func TestServeBlobImageTypeFollowsContent(t *testing.T) {
	a := &App{}
	cache := imgcache.New(t.TempDir(), imgcache.DefaultMaxBytes)
	a.images.once.Do(func() { a.images.cache = cache })

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, img, nil); err != nil {
		t.Fatal(err)
	}
	put := func(blob string, data []byte) {
		if err := cache.Put("blob:acc/"+blob, "application/octet-stream", data); err != nil {
			t.Fatal(err)
		}
	}
	put("photo", jpg.Bytes())
	put("html", []byte("<html><script>alert(1)</script></html>"))
	put("avif", append([]byte{0, 0, 0, 0x1c}, append([]byte("ftypavif"), 0, 0, 0, 0)...))

	serve := func(blob, typ string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		q := url.Values{"account": {"acc"}, "blob": {blob}, "type": {typ}}
		a.serveBlob(w, httptest.NewRequest("GET", BlobPath+"?"+q.Encode(), nil))
		return w
	}

	// 名为 .png、内容是 JPEG: 照样显示, 类型按内容。
	w := serve("photo", "image/png")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" || !bytes.Equal(w.Body.Bytes(), jpg.Bytes()) {
		t.Errorf("jpeg as png: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	// 名为图片、内容不是图片: 不当图片返回。
	if w := serve("html", "image/png"); w.Code != 415 || w.Body.String() == "<html><script>alert(1)</script></html>" {
		t.Errorf("non-image served as image: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	// 标成 PNG、内容是 AVIF: 预览类型表里没有它, 但按内容认得出是位图, 照样显示。
	if w := serve("avif", "image/png"); w.Code != 200 || w.Header().Get("Content-Type") != "image/avif" {
		t.Errorf("avif as png: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
}
