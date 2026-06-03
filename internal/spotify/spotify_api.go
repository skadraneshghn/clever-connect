package spotify

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/logger"
)

// ──────────────────────────────────────────────────────────────────────────────
// Spotify Web API Client — Client Credentials OAuth2 Flow
// ──────────────────────────────────────────────────────────────────────────────

var (
	trackURLRegex  = regexp.MustCompile(`open\.spotify\.com/track/([a-zA-Z0-9]+)`)
	albumURLRegex  = regexp.MustCompile(`open\.spotify\.com/album/([a-zA-Z0-9]+)`)
)

// SpotifyClient handles Spotify Web API interactions
type SpotifyClient struct {
	clientID     string
	clientSecret string
	accessToken  string
	tokenExpiry  time.Time
	httpClient   *http.Client
	mu           sync.Mutex
}

// TrackMeta holds extracted Spotify track metadata (mirrors spotDL's Song dataclass)
type TrackMeta struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Artist      string   `json:"artist"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album"`
	AlbumArtist string   `json:"album_artist"`
	CoverURL    string   `json:"cover_url"`
	ReleaseDate string   `json:"release_date"`
	TrackNumber int      `json:"track_number"`
	TotalTracks int      `json:"total_tracks"`
	DiscNumber  int      `json:"disc_number"`
	DurationMs  int      `json:"duration_ms"`
	ISRC        string   `json:"isrc"`
	Genre       string   `json:"genre"`
	Explicit    bool     `json:"explicit"`
	Popularity  int      `json:"popularity"`
	SpotifyURL  string   `json:"spotify_url"`
}

// AlbumMeta holds album-level metadata
type AlbumMeta struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Artist      string      `json:"artist"`
	CoverURL    string      `json:"cover_url"`
	ReleaseDate string      `json:"release_date"`
	TotalTracks int         `json:"total_tracks"`
	Tracks      []TrackMeta `json:"tracks"`
}

// NewSpotifyClient creates a new Spotify API client
func NewSpotifyClient(clientID, clientSecret string) *SpotifyClient {
	return &SpotifyClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

// authenticate obtains or refreshes the OAuth2 access token
func (sc *SpotifyClient) authenticate() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.accessToken != "" && time.Now().Before(sc.tokenExpiry) {
		return nil
	}

	data := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(sc.clientID, sc.clientSecret)

	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("spotify auth failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	sc.accessToken = result.AccessToken
	sc.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)
	logger.Info("Spotify", "Spotify API authenticated successfully", "expires_in", result.ExpiresIn)
	return nil
}

// apiGet performs an authenticated GET request to the Spotify Web API
func (sc *SpotifyClient) apiGet(endpoint string) (map[string]interface{}, error) {
	if err := sc.authenticate(); err != nil {
		return nil, err
	}

	apiURL := "https://api.spotify.com/v1" + endpoint
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	sc.mu.Lock()
	req.Header.Set("Authorization", "Bearer "+sc.accessToken)
	sc.mu.Unlock()

	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("spotify API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}
	return result, nil
}

// GetTrack fetches full metadata for a single Spotify track (mirrors spotDL Song.from_url)
func (sc *SpotifyClient) GetTrack(trackID string) (*TrackMeta, error) {
	trackData, err := sc.apiGet("/tracks/" + trackID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch track: %w", err)
	}

	// Get artist details for genres
	artists := getStringSlice(trackData, "artists", "name")
	primaryArtistID := ""
	if artistsList, ok := trackData["artists"].([]interface{}); ok && len(artistsList) > 0 {
		if a, ok := artistsList[0].(map[string]interface{}); ok {
			primaryArtistID = getString(a, "id")
		}
	}

	genres := ""
	if primaryArtistID != "" {
		if artistData, err := sc.apiGet("/artists/" + primaryArtistID); err == nil {
			if g, ok := artistData["genres"].([]interface{}); ok && len(g) > 0 {
				if s, ok := g[0].(string); ok {
					genres = s
				}
			}
		}
	}

	// Extract album info
	albumData, _ := trackData["album"].(map[string]interface{})
	albumName := getString(albumData, "name")
	albumArtist := ""
	if albumArtists, ok := albumData["artists"].([]interface{}); ok && len(albumArtists) > 0 {
		if a, ok := albumArtists[0].(map[string]interface{}); ok {
			albumArtist = getString(a, "name")
		}
	}

	// Get highest resolution cover art
	coverURL := getBestImage(albumData)

	// Extract ISRC
	isrc := ""
	if extIDs, ok := trackData["external_ids"].(map[string]interface{}); ok {
		isrc = getString(extIDs, "isrc")
	}

	// Build spotify URL
	spotifyURL := ""
	if extURLs, ok := trackData["external_urls"].(map[string]interface{}); ok {
		spotifyURL = getString(extURLs, "spotify")
	}

	meta := &TrackMeta{
		ID:          getString(trackData, "id"),
		Title:       getString(trackData, "name"),
		Artist:      artists[0],
		Artists:     artists,
		Album:       albumName,
		AlbumArtist: albumArtist,
		CoverURL:    coverURL,
		ReleaseDate: getString(albumData, "release_date"),
		TrackNumber: getInt(trackData, "track_number"),
		TotalTracks: getInt(albumData, "total_tracks"),
		DiscNumber:  getInt(trackData, "disc_number"),
		DurationMs:  getInt(trackData, "duration_ms"),
		ISRC:        isrc,
		Genre:       genres,
		Explicit:    getBool(trackData, "explicit"),
		Popularity:  getInt(trackData, "popularity"),
		SpotifyURL:  spotifyURL,
	}

	return meta, nil
}

// GetAlbum fetches album metadata and all its tracks
func (sc *SpotifyClient) GetAlbum(albumID string) (*AlbumMeta, error) {
	albumData, err := sc.apiGet("/albums/" + albumID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch album: %w", err)
	}

	albumArtist := ""
	if artists, ok := albumData["artists"].([]interface{}); ok && len(artists) > 0 {
		if a, ok := artists[0].(map[string]interface{}); ok {
			albumArtist = getString(a, "name")
		}
	}

	album := &AlbumMeta{
		ID:          getString(albumData, "id"),
		Name:        getString(albumData, "name"),
		Artist:      albumArtist,
		CoverURL:    getBestImage(albumData),
		ReleaseDate: getString(albumData, "release_date"),
		TotalTracks: getInt(albumData, "total_tracks"),
	}

	// Extract tracks from album (paginated)
	if tracksObj, ok := albumData["tracks"].(map[string]interface{}); ok {
		if items, ok := tracksObj["items"].([]interface{}); ok {
			for _, item := range items {
				t, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				trackID := getString(t, "id")
				// Fetch full track metadata for each track (includes ISRC, popularity, etc.)
				trackMeta, err := sc.GetTrack(trackID)
				if err != nil {
					logger.Warn("Spotify", "Failed to fetch track in album", "track_id", trackID, "error", err)
					continue
				}
				album.Tracks = append(album.Tracks, *trackMeta)
			}
		}
	}

	return album, nil
}

// ParseSpotifyURL extracts the type (track/album) and ID from a Spotify URL
func ParseSpotifyURL(rawURL string) (linkType string, id string, err error) {
	if matches := trackURLRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return "track", matches[1], nil
	}
	if matches := albumURLRegex.FindStringSubmatch(rawURL); len(matches) > 1 {
		return "album", matches[1], nil
	}
	return "", "", fmt.Errorf("invalid Spotify URL: must be a track or album link")
}

// ──────────────────────────────────────────────────────────────────────────────
// JSON Helpers
// ──────────────────────────────────────────────────────────────────────────────

func getString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func getBool(m map[string]interface{}, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getStringSlice(m map[string]interface{}, arrayKey, fieldKey string) []string {
	var result []string
	if arr, ok := m[arrayKey].([]interface{}); ok {
		for _, item := range arr {
			if obj, ok := item.(map[string]interface{}); ok {
				if name, ok := obj[fieldKey].(string); ok {
					result = append(result, name)
				}
			}
		}
	}
	if len(result) == 0 {
		result = append(result, "Unknown")
	}
	return result
}

func getBestImage(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	images, ok := m["images"].([]interface{})
	if !ok || len(images) == 0 {
		return ""
	}
	bestURL := ""
	bestSize := 0
	for _, img := range images {
		if imgMap, ok := img.(map[string]interface{}); ok {
			w := getInt(imgMap, "width")
			h := getInt(imgMap, "height")
			if w*h > bestSize {
				bestSize = w * h
				bestURL = getString(imgMap, "url")
			}
		}
	}
	return bestURL
}

// ──────────────────────────────────────────────────────────────────────────────
// Embed Scraping Structures & Function (No-Auth Fallback)
// ──────────────────────────────────────────────────────────────────────────────

type EmbedNextData struct {
	Props struct {
		PageProps struct {
			Status      int    `json:"status"`
			Title       string `json:"title"`
			Description string `json:"description"`
			State       struct {
				Data struct {
					Entity *EmbedEntity `json:"entity"`
				} `json:"data"`
			} `json:"state"`
		} `json:"pageProps"`
	} `json:"props"`
}

type EmbedEntity struct {
	ID          string         `json:"id"`
	URI         string         `json:"uri"`
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Subtitle    string         `json:"subtitle"`
	Description string         `json:"description"`
	Artists     []EmbedArtist  `json:"artists"`
	CoverArt    *EmbedCoverArt `json:"coverArt"`
	Images      []EmbedImage   `json:"images"`
	ReleaseDate *EmbedDate     `json:"releaseDate"`
	Release_Date string        `json:"release_date"`
	TrackList   []EmbedTrack   `json:"trackList"`
	Duration    int            `json:"duration"`
	TrackNumber int            `json:"track_number"`
}

type EmbedArtist struct {
	Name string `json:"name"`
}

type EmbedCoverArt struct {
	Sources []EmbedImage `json:"sources"`
}

type EmbedImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type EmbedDate struct {
	ISOString string `json:"isoString"`
}

type EmbedTrack struct {
	ID          string        `json:"id"`
	URI         string        `json:"uri"`
	Title       string        `json:"title"`
	Name        string        `json:"name"`
	Subtitle    string        `json:"subtitle"`
	Artists     []EmbedArtist `json:"artists"`
	Duration    int           `json:"duration"`
	TrackNumber int           `json:"track_number"`
}

// ScrapeSpotifyEmbed fetches track or album metadata directly from the public Spotify embed player page
func ScrapeSpotifyEmbed(linkType, id string) (interface{}, error) {
	embedURL := fmt.Sprintf("https://open.spotify.com/embed/%s/%s", linkType, id)

	req, err := http.NewRequest("GET", embedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create scrape request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://open.spotify.com/")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scrape request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scrape failed with HTTP status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read scrape response: %w", err)
	}

	body := string(bodyBytes)

	// Extract the __NEXT_DATA__ json script tag
	re := regexp.MustCompile(`<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)
	matches := re.FindStringSubmatch(body)
	if len(matches) < 2 {
		return nil, fmt.Errorf("metadata JSON script block not found in embed page")
	}

	var nextData EmbedNextData
	if err := json.Unmarshal([]byte(matches[1]), &nextData); err != nil {
		return nil, fmt.Errorf("failed to parse page metadata JSON: %w", err)
	}

	if nextData.Props.PageProps.Status == 404 {
		return nil, fmt.Errorf("spotify embed player returned 404 (often blocked on VPS/cloud hosts by Spotify's CDN)")
	}

	entity := nextData.Props.PageProps.State.Data.Entity
	if entity == nil {
		return nil, fmt.Errorf("no metadata entity found in page payload")
	}

	if entity.Name == "" {
		return nil, fmt.Errorf("metadata entity name is empty")
	}

	// Normalize cover image URL
	coverURL := ""
	if entity.CoverArt != nil && len(entity.CoverArt.Sources) > 0 {
		coverURL = entity.CoverArt.Sources[0].URL
	} else if len(entity.Images) > 0 {
		coverURL = entity.Images[0].URL
	}

	// Normalize release date
	releaseDate := entity.Release_Date
	if entity.ReleaseDate != nil && entity.ReleaseDate.ISOString != "" {
		releaseDate = entity.ReleaseDate.ISOString
	}

	// Normalize artists
	var artists []string
	for _, a := range entity.Artists {
		if a.Name != "" {
			artists = append(artists, a.Name)
		}
	}

	// Fallback to subtitle if artists list is empty (Spotify embed album pages use subtitle)
	if len(artists) == 0 && entity.Subtitle != "" {
		parts := strings.Split(entity.Subtitle, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				artists = append(artists, trimmed)
			}
		}
	}

	primaryArtist := "Unknown Artist"
	if len(artists) > 0 {
		primaryArtist = artists[0]
	} else {
		artists = []string{primaryArtist}
	}

	switch linkType {
	case "track":
		// Fallback to subtitle if artists is empty/unknown but subtitle is present
		if len(artists) == 1 && artists[0] == "Unknown Artist" && entity.Subtitle != "" {
			var trackArtists []string
			parts := strings.Split(entity.Subtitle, ",")
			for _, part := range parts {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					trackArtists = append(trackArtists, trimmed)
				}
			}
			if len(trackArtists) > 0 {
				artists = trackArtists
				primaryArtist = artists[0]
			}
		}

		meta := &TrackMeta{
			ID:          id,
			Title:       entity.Name,
			Artist:      primaryArtist,
			Artists:     artists,
			Album:       "Unknown Album",
			AlbumArtist: primaryArtist,
			CoverURL:    coverURL,
			ReleaseDate: releaseDate,
			TrackNumber: entity.TrackNumber,
			TotalTracks: 1,
			DiscNumber:  1,
			DurationMs:  entity.Duration,
			SpotifyURL:  fmt.Sprintf("https://open.spotify.com/track/%s", id),
		}
		return meta, nil

	case "album":
		album := &AlbumMeta{
			ID:          id,
			Name:        entity.Name,
			Artist:      primaryArtist,
			CoverURL:    coverURL,
			ReleaseDate: releaseDate,
			TotalTracks: len(entity.TrackList),
		}

		for idx, t := range entity.TrackList {
			tTrackID := t.ID
			if tTrackID == "" {
				parts := strings.Split(t.URI, ":")
				if len(parts) > 2 {
					tTrackID = parts[2]
				} else {
					tTrackID = fmt.Sprintf("%s_track_%d", id, idx)
				}
			}

			tTitle := t.Title
			if tTitle == "" {
				tTitle = t.Name
			}

			var tArtists []string
			for _, a := range t.Artists {
				if a.Name != "" {
					tArtists = append(tArtists, a.Name)
				}
			}

			// Fallback to track subtitle if t.Artists is empty (essential for scraped album tracks)
			if len(tArtists) == 0 && t.Subtitle != "" {
				parts := strings.Split(t.Subtitle, ",")
				for _, part := range parts {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						tArtists = append(tArtists, trimmed)
					}
				}
			}

			tPrimaryArtist := primaryArtist
			if len(tArtists) > 0 {
				tPrimaryArtist = tArtists[0]
			} else {
				tArtists = []string{tPrimaryArtist}
			}

			tNum := t.TrackNumber
			if tNum == 0 {
				tNum = idx + 1
			}

			album.Tracks = append(album.Tracks, TrackMeta{
				ID:          tTrackID,
				Title:       tTitle,
				Artist:      tPrimaryArtist,
				Artists:     tArtists,
				Album:       entity.Name,
				AlbumArtist: primaryArtist,
				CoverURL:    coverURL,
				ReleaseDate: releaseDate,
				TrackNumber: tNum,
				TotalTracks: len(entity.TrackList),
				DiscNumber:  1,
				DurationMs:  t.Duration,
				SpotifyURL:  fmt.Sprintf("https://open.spotify.com/track/%s", tTrackID),
			})
		}
		return album, nil
	}

	return nil, fmt.Errorf("unsupported Spotify link type for scraping: %s", linkType)
}

