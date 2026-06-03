package downloader

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseContentDispositionFilename(t *testing.T) {
	tests := []struct {
		disp     string
		expected string
	}{
		{
			disp:     `attachment; filename="standard.txt"`,
			expected: "standard.txt",
		},
		{
			disp:     `attachment; filename=standard.txt`,
			expected: "standard.txt",
		},
		{
			disp:     `attachment; filename*=UTF-8''%e2%82%ac%20rates.pdf`,
			expected: "€ rates.pdf",
		},
		{
			disp:     `attachment; filename*=UTF-8''www.0xxx.ws_somefile.mp4`,
			expected: "www.0xxx.ws_somefile.mp4",
		},
		{
			disp:     `attachment; filename="fallback.txt"; filename*=UTF-8''preferred.txt`,
			expected: "preferred.txt", // filename* takes precedence
		},
		{
			disp:     `inline; filename*=UTF-8''hello%20world.txt`,
			expected: "hello world.txt",
		},
		{
			disp:     `form-data; name="file"; filename="foo.bar"`,
			expected: "foo.bar",
		},
	}

	for _, tc := range tests {
		t.Run(tc.disp, func(t *testing.T) {
			got := parseContentDispositionFilename(tc.disp)
			if got != tc.expected {
				t.Errorf("parseContentDispositionFilename(%q) = %q; want %q", tc.disp, got, tc.expected)
			}
		})
	}
}

func TestResolvePremiumURL(t *testing.T) {
	// Start a mock HTTP server to simulate the redirect chain and Content-Disposition headers
	var steps []func(w http.ResponseWriter, r *http.Request)
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(steps) == 0 {
			http.Error(w, "unexpected request", http.StatusInternalServerError)
			return
		}
		step := steps[0]
		steps = steps[1:]
		step(w, r)
	}))
	defer server.Close()

	// Scenario 1: Initial URL redirects to a CDN URL, which serves the file with Content-Disposition
	steps = []func(w http.ResponseWriter, r *http.Request){
		// First request (to premium.to api) -> redirects to CDN
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", server.URL+"/cdn-link")
			w.WriteHeader(http.StatusFound)
		},
		// Second request (to CDN) -> returns 200 with Content-Disposition
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''file%20resolved.zip`)
			w.WriteHeader(http.StatusOK)
		},
	}

	client := server.Client()
	resolvedURL, filename, err := resolvePremiumURL(server.URL+"/api-call", client)
	if err != nil {
		t.Fatalf("resolvePremiumURL failed: %v", err)
	}

	expectedURL := server.URL + "/cdn-link"
	if resolvedURL != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, resolvedURL)
	}
	expectedFilename := "file resolved.zip"
	if filename != expectedFilename {
		t.Errorf("expected filename %q, got %q", expectedFilename, filename)
	}

	// Scenario 2: First request returns a URL directly in the plain text body (some premium.to endpoints do this)
	steps = []func(w http.ResponseWriter, r *http.Request){
		// First request -> 200 OK with URL in body
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(server.URL + "/direct-cdn"))
		},
		// Second request -> redirects once more to final destination
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", server.URL+"/final-destination")
			w.WriteHeader(http.StatusMovedPermanently)
		},
		// Third request -> 200 OK with Content-Disposition
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Disposition", `attachment; filename="final_file.tar.gz"`)
			w.WriteHeader(http.StatusOK)
		},
	}

	resolvedURL, filename, err = resolvePremiumURL(server.URL+"/api-call", client)
	if err != nil {
		t.Fatalf("resolvePremiumURL failed: %v", err)
	}

	expectedURL = server.URL + "/final-destination"
	if resolvedURL != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, resolvedURL)
	}
	expectedFilename = "final_file.tar.gz"
	if filename != expectedFilename {
		t.Errorf("expected filename %q, got %q", expectedFilename, filename)
	}

	// Scenario 3: First request returns JSON error
	steps = []func(w http.ResponseWriter, r *http.Request){
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"err":"Premium account expired"}`))
		},
	}

	_, _, err = resolvePremiumURL(server.URL+"/api-call", client)
	if err == nil {
		t.Fatal("expected error for JSON error response, got nil")
	}
	if !urlErrorContains(err, "Premium account expired") {
		t.Errorf("expected error message to contain 'Premium account expired', got %v", err)
	}
}

func urlErrorContains(err error, msg string) bool {
	if err == nil {
		return false
	}
	return (err.Error() == msg) || (len(err.Error()) > 0 && (fmt.Sprintf("%v", err) != "" && (fmt.Sprintf("%v", err) != msg)))
}
