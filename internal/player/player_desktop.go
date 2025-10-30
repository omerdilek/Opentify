//go:build !android && !ios

package player

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"
)

var (
	speakerOnce sync.Once
	// Use a fixed speaker sample rate and resample inputs to avoid reinitializing the audio device.
	speakerSR = beep.SampleRate(44100)
)

type Player struct {
	mu      sync.Mutex
	stream  beep.StreamSeekCloser // original decoder stream (seekable)
	play    beep.Streamer         // resampled wrapper used for actual playback
	vol     *effects.Volume       // volume wrapper
	volNorm float64               // [0..1]
	ctrl    *beep.Ctrl
	sr      beep.SampleRate // original file's sample rate
	started bool
	current string
}

func New() *Player { return &Player{volNorm: 1} }

func (p *Player) volDB() float64 {
	// Map normalized [0..1] to dB/10 range [-4..0] (i.e., -40dB to 0dB)
	v := p.volNorm
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return -4 + 4*v
}

// SetVolume sets volume with normalized value in [0,1]. 0 is near silent, 1 is 0dB.
func (p *Player) SetVolume(norm float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volNorm = norm
	if p.vol != nil {
		p.vol.Base = 10
		p.vol.Volume = p.volDB()
	}
}

// Volume returns the normalized volume [0,1].
func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volNorm
}

func (p *Player) decodeFile(path string) (beep.StreamSeekCloser, beep.Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, beep.Format{}, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3":
		st, format, err := mp3.Decode(f)
		if err != nil {
			_ = f.Close()
		}
		return st, format, err
	case ".wav":
		st, format, err := wav.Decode(f)
		if err != nil {
			_ = f.Close()
		}
		return st, format, err
	case ".flac":
		st, format, err := flac.Decode(f)
		if err != nil {
			_ = f.Close()
		}
		return st, format, err
	case ".ogg":
		st, format, err := vorbis.Decode(f)
		if err != nil {
			_ = f.Close()
		}
		return st, format, err
	default:
		_ = f.Close()
		return nil, beep.Format{}, fmt.Errorf("desteklenmeyen format: %s", ext)
	}
}

func ensureSpeaker() {
	speakerOnce.Do(func() {
		_ = speaker.Init(speakerSR, speakerSR.N(time.Second/10))
	})
}

func (p *Player) Load(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop current playback and release previous stream
	if p.stream != nil {
		if p.ctrl != nil {
			speaker.Lock()
			p.ctrl.Paused = true
			speaker.Unlock()
		}
		_ = p.stream.Close()
		p.stream = nil
		p.play = nil
		p.ctrl = nil
	}

	st, format, err := p.decodeFile(path)
	if err != nil {
		return err
	}
	p.stream = st
	p.sr = format.SampleRate

	ensureSpeaker()

	// Prepare resampled playback stream to match fixed speaker sample rate
	p.play = beep.Resample(4, p.sr, speakerSR, p.stream)
	p.vol = &effects.Volume{Streamer: p.play, Base: 10, Volume: p.volDB()}
	p.ctrl = &beep.Ctrl{Streamer: p.vol, Paused: true}

	// Ensure no stale streamers remain in the mixer (single-player app)
	speaker.Clear()

	p.started = false
	p.current = path
	return nil
}

func (p *Player) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil || p.ctrl == nil {
		return
	}
	if !p.started {
		p.started = true
		speaker.Play(p.ctrl)
	}
	speaker.Lock()
	p.ctrl.Paused = false
	speaker.Unlock()
}

func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctrl != nil {
		speaker.Lock()
		p.ctrl.Paused = true
		speaker.Unlock()
	}
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil || p.ctrl == nil {
		return
	}
	speaker.Lock()
	p.ctrl.Paused = true
	speaker.Unlock()
	speaker.Lock()
	defer speaker.Unlock()
	_ = p.stream.Seek(0)
	// Reset resampler state after seek-to-start
	p.play = beep.Resample(4, p.sr, speakerSR, p.stream)
	p.vol = &effects.Volume{Streamer: p.play, Base: 10, Volume: p.volDB()}
	p.ctrl.Streamer = p.vol
	p.started = false
}

func (p *Player) CurrentFile() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == "" {
		return "", errors.New("parça yok")
	}
	return p.current, nil
}

// IsPlaying reports whether audio is currently playing (not paused and started).
func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.started && p.ctrl != nil && !p.ctrl.Paused
}

// Duration returns total duration of the loaded track.
func (p *Player) Duration() (time.Duration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil {
		return 0, errors.New("akış yok")
	}
	l := p.stream.Len()
	if l <= 0 || p.sr == 0 {
		return 0, errors.New("uzunluk bilinmiyor")
	}
	return time.Duration(float64(time.Second) * float64(l) / float64(p.sr)), nil
}

// Position returns current playback position.
func (p *Player) Position() (time.Duration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil || p.sr == 0 {
		return 0, errors.New("akış yok")
	}
	pos := p.stream.Position()
	return time.Duration(float64(time.Second) * float64(pos) / float64(p.sr)), nil
}

// SeekRatio seeks to given ratio [0,1] of the track length.
func (p *Player) SeekRatio(r float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil {
		return errors.New("akış yok")
	}
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	l := p.stream.Len()
	if l <= 0 {
		return errors.New("uzunluk bilinmiyor")
	}
	// Some decoders (e.g., mp3) panic if seeking to exactly l; clamp to [0, l-1]
	target := int(float64(l-1) * r)
	if target < 0 {
		target = 0
	}
	if target >= l {
		target = l - 1
	}
	speaker.Lock()
	defer speaker.Unlock()
	if err := p.stream.Seek(target); err != nil {
		return err
	}
	// Reset resampler after seek
	p.play = beep.Resample(4, p.sr, speakerSR, p.stream)
	p.vol = &effects.Volume{Streamer: p.play, Base: 10, Volume: p.volDB()}
	if p.ctrl != nil {
		p.ctrl.Streamer = p.vol
	}
	return nil
}

// SeekBy moves relative by the given duration (positive or negative).
func (p *Player) SeekBy(d time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil || p.sr == 0 {
		return errors.New("akış yok")
	}
	cur := p.stream.Position()
	delta := int(float64(p.sr) * d.Seconds())
	target := cur + delta
	if target < 0 {
		target = 0
	}
	l := p.stream.Len()
	if l > 0 && target >= l {
		target = l - 1
	}
	speaker.Lock()
	defer speaker.Unlock()
	if err := p.stream.Seek(target); err != nil {
		return err
	}
	p.play = beep.Resample(4, p.sr, speakerSR, p.stream)
	p.vol = &effects.Volume{Streamer: p.play, Base: 10, Volume: p.volDB()}
	if p.ctrl != nil {
		p.ctrl.Streamer = p.vol
	}
	return nil
}

// SeekTo moves to the absolute position given by duration.
func (p *Player) SeekTo(d time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil || p.sr == 0 {
		return errors.New("akış yok")
	}
	target := int(float64(p.sr) * d.Seconds())
	if target < 0 {
		target = 0
	}
	l := p.stream.Len()
	if l > 0 && target >= l {
		target = l - 1
	}
	speaker.Lock()
	defer speaker.Unlock()
	if err := p.stream.Seek(target); err != nil {
		return err
	}
	p.play = beep.Resample(4, p.sr, speakerSR, p.stream)
	p.vol = &effects.Volume{Streamer: p.play, Base: 10, Volume: p.volDB()}
	if p.ctrl != nil {
		p.ctrl.Streamer = p.vol
	}
	return nil
}
