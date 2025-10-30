//go:build !vlc

package video

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Player struct{}

func New() (*Player, error)                        { return &Player{}, nil }
func (v *Player) Load(path string) error           { return nil }
func (v *Player) Play() error                      { return nil }
func (v *Player) Pause() error                     { return nil }
func (v *Player) Resume() error                    { return nil }
func (v *Player) Stop() error                      { return nil }
func (v *Player) IsPlaying() bool                  { return false }
func (v *Player) Current() string                  { return "" }
func (v *Player) Release()                         {}
func (v *Player) SetVolume(vol float64) error      { return nil }
func (v *Player) SetPosition(ratio float64) error  { return nil }
func (v *Player) Position() (time.Duration, error) { return 0, nil }
func (v *Player) Duration() (time.Duration, error) { return 0, nil }

// Visual returns a placeholder while VLC tag is disabled.
func (v *Player) Visual() fyne.CanvasObject {
	return container.NewCenter(widget.NewLabel("Video (mp4) oynatma için libVLC ile derleyin: -tags vlc"))
}
