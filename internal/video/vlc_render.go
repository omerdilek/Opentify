//go:build vlc && !android && !ios

package video

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os/exec"
	"sync"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	vlc "github.com/adrg/libvlc-go/v3"
)

// Player with libVLC-backed audio and ffmpeg for video frames.
type Player struct {
	p       *vlc.Player
	current string

	mu       sync.RWMutex
	w, h     uint
	frame    *image.RGBA
	img      *canvas.Image
	stopped  bool
	frameCtx chan struct{}
}

func New() (*Player, error) {
	// Initialize VLC with options to disable video output window
	if err := vlc.Init("--no-video", "--quiet"); err != nil {
		return nil, err
	}
	p, err := vlc.NewPlayer()
	if err != nil {
		return nil, err
	}
	v := &Player{p: p, stopped: true}
	v.img = canvas.NewImageFromImage(placeholderImage())
	v.img.FillMode = canvas.ImageFillContain
	v.img.SetMinSize(fyne.NewSize(320, 240))
	return v, nil
}

func (v *Player) Load(path string) error {
	// Properly stop previous playback
	if v.current != "" {
		v.Stop()
		// Wait a bit for cleanup
		time.Sleep(50 * time.Millisecond)
	}

	m, err := vlc.NewMediaFromPath(path)
	if err != nil {
		return err
	}
	if err := v.p.SetMedia(m); err != nil {
		m.Release()
		return err
	}
	m.Release()
	v.current = path
	v.stopped = true

	// Reset to placeholder
	fyne.Do(func() {
		v.mu.Lock()
		v.img.Image = placeholderImage()
		v.img.Refresh()
		v.mu.Unlock()
	})

	return nil
}

func (v *Player) Play() error {
	// Stop any existing frame extraction
	if v.frameCtx != nil {
		close(v.frameCtx)
		v.frameCtx = nil
		time.Sleep(50 * time.Millisecond) // wait for goroutine to stop
	}

	if err := v.p.Play(); err != nil {
		return err
	}
	v.stopped = false
	// Start frame extraction goroutine
	v.frameCtx = make(chan struct{})
	go v.extractFrames()
	return nil
}

func (v *Player) Pause() error  { return v.p.SetPause(true) }
func (v *Player) Resume() error { return v.p.SetPause(false) }

func (v *Player) Stop() error {
	v.stopped = true
	if v.frameCtx != nil {
		select {
		case <-v.frameCtx:
			// already closed
		default:
			close(v.frameCtx)
		}
		v.frameCtx = nil
	}

	err := v.p.Stop()

	// Reset to placeholder
	fyne.Do(func() {
		v.mu.Lock()
		v.img.Image = placeholderImage()
		v.img.Refresh()
		v.mu.Unlock()
	})

	return err
}

func (v *Player) IsPlaying() bool {
	_, _ = v.p.MediaState()
	return v.p.IsPlaying()
}

func (v *Player) Current() string { return v.current }

func (v *Player) SetVolume(vol float64) error {
	// VLC volume is 0-100
	return v.p.SetVolume(int(vol * 100))
}

func (v *Player) SetPosition(ratio float64) error {
	return v.p.SetMediaPosition(float32(ratio))
}

func (v *Player) Position() (time.Duration, error) {
	ms, err := v.p.MediaTime()
	if err != nil {
		return 0, err
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func (v *Player) Duration() (time.Duration, error) {
	ms, err := v.p.MediaLength()
	if err != nil {
		return 0, err
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func (v *Player) Release() {
	v.Stop()
	v.p.Release()
}

func (v *Player) Visual() fyne.CanvasObject {
	// Wrap in a container to allow layout
	return container.NewMax(v.img)
}

// libVLC video callbacks
func (v *Player) videoFormat(chroma string, width, height, pitches, lines *uint) unsafe.Pointer {
	// Request RV32 (RGBA)
	*((*[4]byte)(unsafe.Pointer(&chroma))) = [4]byte{'R', 'V', '3', '2'}
	v.mu.Lock()
	v.w, v.h = *width, *height
	buf := image.NewRGBA(image.Rect(0, 0, int(*width), int(*height)))
	v.frame = buf
	v.mu.Unlock()
	*pitches = uint(buf.Stride)
	*lines = *height
	return unsafe.Pointer(&v.frame.Pix[0])
}

func (v *Player) videoCleanup() { /* no-op */ }

func (v *Player) lock(id *unsafe.Pointer) {
	v.mu.RLock()
	if v.frame != nil {
		*id = unsafe.Pointer(&v.frame.Pix[0])
	}
}
func (v *Player) unlock(id *unsafe.Pointer) { v.mu.RUnlock() }

func (v *Player) display(id unsafe.Pointer) {
	v.mu.RLock()
	if v.frame != nil {
		img := image.NewRGBA(v.frame.Bounds())
		draw.Draw(img, img.Bounds(), v.frame, image.Point{}, draw.Src)
		v.img.Image = img
		v.img.Refresh()
	}
	v.mu.RUnlock()
}

func placeholderImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{20, 20, 20, 255}}, image.Point{}, draw.Src)
	return img
}

// extractFrames periodically captures video frames using ffmpeg and updates the canvas
func (v *Player) extractFrames() {
	ticker := time.NewTicker(200 * time.Millisecond) // ~5 fps (lower to reduce CPU)
	defer ticker.Stop()

	for {
		select {
		case <-v.frameCtx:
			return
		case <-ticker.C:
			if v.stopped || !v.p.IsPlaying() {
				continue
			}
			// Get current playback position
			pos, err := v.p.MediaTime()
			if err != nil || pos < 0 {
				continue
			}
			// Extract frame at current position using ffmpeg
			if frame := v.captureFrame(float64(pos) / 1000.0); frame != nil {
				fyne.Do(func() {
					v.mu.Lock()
					v.img.Image = frame
					v.img.Refresh()
					v.mu.Unlock()
				})
			}
		}
	}
}

// captureFrame extracts a single frame at given timestamp using ffmpeg
func (v *Player) captureFrame(seconds float64) image.Image {
	if v.current == "" {
		return nil
	}

	// ffmpeg command to extract frame at specific time
	// -ss before -i for faster seeking
	cmd := exec.Command("ffmpeg",
		"-loglevel", "quiet",
		"-ss", fmt.Sprintf("%.3f", seconds),
		"-i", v.current,
		"-vframes", "1",
		"-vf", "scale=640:-1", // scale down for performance
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-q:v", "8", // lower quality for faster processing
		"-")

	var buf bytes.Buffer
	cmd.Stdout = &buf

	if err := cmd.Run(); err != nil {
		return nil
	}

	img, err := jpeg.Decode(&buf)
	if err != nil {
		return nil
	}

	return img
}
