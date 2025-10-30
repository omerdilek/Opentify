package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Info holds minimal track metadata and artwork URL.
type Info struct {
	Title   string
	Artist  string
	Album   string
	Artwork string // URL
}

// Lookup queries iTunes Search API for a given freeform term and returns the best match.
// No API key required.
func Lookup(ctx context.Context, term string) (Info, error) {
	q := url.Values{}
	q.Set("term", term)
	q.Set("entity", "song")
	q.Set("limit", "1")
	endpoint := "https://itunes.apple.com/search?" + q.Encode()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	var out struct {
		ResultCount int `json:"resultCount"`
		Results     []struct {
			TrackName      string `json:"trackName"`
			ArtistName     string `json:"artistName"`
			CollectionName string `json:"collectionName"`
			ArtworkURL100  string `json:"artworkUrl100"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Info{}, err
	}
	if out.ResultCount == 0 || len(out.Results) == 0 {
		return Info{}, fmt.Errorf("no results")
	}
	r := out.Results[0]
	art := r.ArtworkURL100
	// Request higher-res artwork if available
	art = strings.Replace(art, "100x100bb.jpg", "600x600bb.jpg", 1)
	return Info{
		Title:   r.TrackName,
		Artist:  r.ArtistName,
		Album:   r.CollectionName,
		Artwork: art,
	}, nil
}
