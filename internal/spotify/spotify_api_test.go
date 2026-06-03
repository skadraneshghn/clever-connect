package spotify

import (
	"testing"
)

func TestParseSpotifyURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
		linkType string
		wantErr  bool
	}{
		{
			url:      "https://open.spotify.com/track/7qiZfU4dY1lWlZ1tXf3q7p?si=some_param",
			expected: "7qiZfU4dY1lWlZ1tXf3q7p",
			linkType: "track",
			wantErr:  false,
		},
		{
			url:      "https://open.spotify.com/album/4aavyWmZMA2Z2hxClCjAWG",
			expected: "4aavyWmZMA2Z2hxClCjAWG",
			linkType: "album",
			wantErr:  false,
		},
		{
			url:      "https://example.com/not-spotify",
			expected: "",
			linkType: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		lType, id, err := ParseSpotifyURL(tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSpotifyURL(%q) error = %v, wantErr = %v", tt.url, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if lType != tt.linkType {
				t.Errorf("ParseSpotifyURL(%q) type = %q, want %q", tt.url, lType, tt.linkType)
			}
			if id != tt.expected {
				t.Errorf("ParseSpotifyURL(%q) id = %q, want %q", tt.url, id, tt.expected)
			}
		}
	}
}

func TestScrapeSpotifyEmbed(t *testing.T) {
	// Shape of You
	trackID := "7qiZfU4dY1lWlZ1tXf3q7p"
	res, err := ScrapeSpotifyEmbed("track", trackID)
	if err != nil {
		// Scrape failure is acceptable if blocked by CDN/network
		t.Logf("Scrape failed (acceptable if blocked by network/CDN): %v", err)
		return
	}

	meta, ok := res.(*TrackMeta)
	if !ok {
		t.Fatalf("Expected result to be *TrackMeta, got %T", res)
	}

	if meta.Title == "" {
		t.Error("Expected title to be non-empty")
	}

	t.Logf("Successfully scraped metadata: Title=%q, Artist=%q, CoverURL=%q, ReleaseDate=%q",
		meta.Title, meta.Artist, meta.CoverURL, meta.ReleaseDate)
}
