//go:build android || ios

package player

import "time"

// Basit mobil iskelet; gerçek oynatma ileride eklenecek.

type Player struct{}

func New() *Player { return &Player{} }

func (p *Player) Load(path string) error       { return nil }
func (p *Player) Play()                        {}
func (p *Player) Pause()                       {}
func (p *Player) Stop()                        {}
func (p *Player) CurrentFile() (string, error) { return "", nil }

// Stubs for desktop-only helpers
func (p *Player) IsPlaying() bool                  { return false }
func (p *Player) Duration() (time.Duration, error) { return 0, nil }
func (p *Player) Position() (time.Duration, error) { return 0, nil }
func (p *Player) SeekRatio(r float64) error        { return nil }
func (p *Player) SeekBy(d time.Duration) error     { return nil }
func (p *Player) SeekTo(d time.Duration) error     { return nil }
