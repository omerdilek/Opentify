package streaming

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Track represents an online music track
type Track struct {
	ID        string
	Title     string
	Artist    string
	Duration  string
	URL       string
	IsLocal   bool
	LocalPath string
	HasVideo  bool // Whether this is a video (music video)
}

// SearchYouTube searches for music on YouTube using yt-dlp
func SearchYouTube(query string, limit int) ([]Track, error) {
	// Check if yt-dlp is installed
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return nil, fmt.Errorf("yt-dlp not installed: %w", err)
	}

	// Search YouTube
	cmd := exec.Command("yt-dlp",
		"--dump-json",
		"--skip-download",
		"--no-playlist",
		"--flat-playlist",
		fmt.Sprintf("ytsearch%d:%s", limit, query),
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Parse results (yt-dlp outputs one JSON object per line)
	var tracks []Track
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var result struct {
			ID         string  `json:"id"`
			Title      string  `json:"title"`
			Channel    string  `json:"channel"`
			Uploader   string  `json:"uploader"`
			Duration   float64 `json:"duration"`
			URL        string  `json:"url"`
			WebpageURL string  `json:"webpage_url"`
		}

		if err := json.Unmarshal([]byte(line), &result); err != nil {
			continue
		}

		// Skip if no ID
		if result.ID == "" {
			continue
		}

		// Build URL if not present
		videoURL := result.WebpageURL
		if videoURL == "" && result.URL != "" {
			videoURL = result.URL
		}
		if videoURL == "" {
			videoURL = "https://www.youtube.com/watch?v=" + result.ID
		}

		// Get artist name
		artist := result.Channel
		if artist == "" {
			artist = result.Uploader
		}
		if artist == "" {
			artist = "Unknown"
		}

		// Format duration
		duration := "0:00"
		if result.Duration > 0 {
			minutes := int(result.Duration) / 60
			seconds := int(result.Duration) % 60
			duration = fmt.Sprintf("%d:%02d", minutes, seconds)
		}

		tracks = append(tracks, Track{
			ID:       result.ID,
			Title:    result.Title,
			Artist:   artist,
			Duration: duration,
			URL:      videoURL,
			IsLocal:  false,
			HasVideo: true, // YouTube results have video by default
		})
	}

	return tracks, nil
}

// Download downloads a track to the specified directory
func Download(track Track, destDir string) (string, error) {
	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	// Sanitize filename
	safeName := sanitizeFilename(track.Title)
	outputTemplate := filepath.Join(destDir, safeName+".%(ext)s")

	// Download with yt-dlp
	cmd := exec.Command("yt-dlp",
		"-x", // Extract audio only
		"--audio-format", "mp3",
		"--audio-quality", "0", // Best quality
		"-o", outputTemplate,
		"--no-playlist",
		track.URL,
	)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	// Return the actual file path
	outputPath := filepath.Join(destDir, safeName+".mp3")
	return outputPath, nil
}

// GetStreamURL gets a direct stream URL (for immediate playback without download)
func GetStreamURL(track Track) (string, error) {
	cmd := exec.Command("yt-dlp",
		"-g", // Get URL only
		"--format", "bestaudio",
		track.URL,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get stream URL: %w", err)
	}

	streamURL := strings.TrimSpace(string(output))
	return streamURL, nil
}

// DownloadVideo downloads a track as video (MP4) to the specified directory
func DownloadVideo(track Track, destDir string) (string, error) {
	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	// Sanitize filename
	safeName := sanitizeFilename(track.Title)
	outputTemplate := filepath.Join(destDir, safeName+".%(ext)s")

	// Download video with yt-dlp
	cmd := exec.Command("yt-dlp",
		"--format", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--merge-output-format", "mp4",
		"-o", outputTemplate,
		"--no-playlist",
		track.URL,
	)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("video download failed: %w", err)
	}

	// Return the actual file path
	outputPath := filepath.Join(destDir, safeName+".mp4")
	return outputPath, nil
}

// IsDownloaded checks if a track is already downloaded
func IsDownloaded(track Track, musicDir string) (bool, string) {
	safeName := sanitizeFilename(track.Title)
	path := filepath.Join(musicDir, safeName+".mp3")

	if _, err := os.Stat(path); err == nil {
		return true, path
	}

	return false, ""
}

// IsDownloadedVideo checks if a video track is already downloaded
func IsDownloadedVideo(track Track, musicDir string) (bool, string) {
	safeName := sanitizeFilename(track.Title)
	path := filepath.Join(musicDir, safeName+".mp4")

	if _, err := os.Stat(path); err == nil {
		return true, path
	}

	return false, ""
}

// sanitizeFilename removes invalid characters from filename
func sanitizeFilename(name string) string {
	// Replace invalid characters
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "",
		"\"", "'",
		"<", "-",
		">", "-",
		"|", "-",
	)

	cleaned := replacer.Replace(name)

	// Limit length
	if len(cleaned) > 200 {
		cleaned = cleaned[:200]
	}

	return strings.TrimSpace(cleaned)
}
