package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type State struct {
	Playlists map[string][]string `json:"playlists"`
	Liked     map[string]bool     `json:"liked"`
	Settings  Settings            `json:"settings"`
}

type Settings struct {
	DownloadFormat string `json:"download_format"` // "mp3" or "mp4"
	Theme          string `json:"theme"`           // "light" or "dark"
}

func Default() *State {
	return &State{
		Playlists: map[string][]string{},
		Liked:     map[string]bool{},
		Settings: Settings{
			DownloadFormat: "mp3",
			Theme:          "light",
		},
	}
}

func EnsureDir(path string) error {
	d := filepath.Dir(path)
	if d == "." || d == "" {
		return nil
	}
	return os.MkdirAll(d, 0o755)
}

func Load(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.Playlists == nil {
		s.Playlists = map[string][]string{}
	}
	if s.Liked == nil {
		s.Liked = map[string]bool{}
	}
	// Defaults for settings
	if s.Settings.DownloadFormat != "mp3" && s.Settings.DownloadFormat != "mp4" {
		s.Settings.DownloadFormat = "mp3"
	}
	if s.Settings.Theme != "dark" && s.Settings.Theme != "light" {
		s.Settings.Theme = "light"
	}
	return &s, nil
}

func Save(path string, s *State) error {
	if err := EnsureDir(path); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
