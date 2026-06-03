package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUploadFavicon(t *testing.T) {
	// Create data dir if not exists (so handler doesn't fail on os.Create)
	_ = os.MkdirAll("data", 0755)
	defer os.RemoveAll("data/favicon.png")

	// Set gin to test mode
	gin.SetMode(gin.TestMode)

	// Create router and register the endpoint
	router := gin.New()
	router.POST("/api/settings/favicon", UploadFavicon)

	// Create a dummy 100x100 PNG image in memory
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{0, 0, 255, 255})
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	if err != nil {
		t.Fatalf("failed to encode dummy PNG: %v", err)
	}

	// Prepare multipart request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("favicon", "test.png")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write(buf.Bytes())
	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", "/api/settings/favicon", body)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Record response
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
}
