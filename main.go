package main

import (
	"context"
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"opentify/internal/discord"
	"opentify/internal/meta"
	"opentify/internal/player"
	"opentify/internal/state"
	"opentify/internal/streaming"
	"opentify/internal/video"
)

func ensureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	return nil
}

// loadResource loads a local file into a Fyne resource (returns nil on error).
func loadResource(path string) fyne.Resource {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return fyne.NewStaticResource(filepath.Base(path), b)
}

func isMedia(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3", ".wav", ".flac", ".ogg", ".mp4":
		return true
	}
	return false
}

func scanMusic(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && isMedia(p) {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func main() {
	const dbDir = "musicdb"
	_ = ensureDir(dbDir)

	a := app.New()
	w := a.NewWindow("Opentify")
	w.Resize(fyne.NewSize(1080, 720))

	// Set app/window icon from logo-mini.png if present
	if res := loadResource("logo-mini.png"); res != nil {
		a.SetIcon(res)
		w.SetIcon(res)
	}

	p := player.New()

	// Initialize Discord Rich Presence
	dc := discord.Get()
	if err := dc.Connect(); err != nil {
		// Non-fatal: just log and continue without Discord integration
		fmt.Fprintf(os.Stderr, "Discord bağlantısı kurulamadı: %v\n", err)
	}
	defer dc.Disconnect()

	files, err := scanMusic(dbDir)
	if err != nil {
		dialog.ShowError(err, w)
	}

	st, _ := state.Load("data/state.json")
	_ = state.EnsureDir("data/state.json")

	// Apply theme from settings
	if strings.ToLower(st.Settings.Theme) == "dark" {
		a.Settings().SetTheme(theme.DarkTheme())
	} else {
		a.Settings().SetTheme(theme.LightTheme())
	}

	currentPage := "Keşfet"
	currentPlaylist := ""

	var selected string
	var _ *streaming.Track // selectedOnlineTrack - reserved for future use
	var onlineTracks []streaming.Track
	view := append([]string(nil), files...)
	showingOnline := false
	// UI controls referenced before initialization
	var currentTrack *widget.Label
	var toggleBtn *widget.Button
	var posLabel, durLabel *widget.Label
	var progress *widget.Slider
	var cover *canvas.Image
	var titleLbl, artistLbl, albumLbl *widget.Label
	var visualShow func(showVideo bool)
	var vplayer *video.Player
	var videoBox *fyne.Container
	var suppressSelect bool
	var applyView func()
	var refreshPlaylists func()

	list := widget.NewList(
		func() int {
			if showingOnline {
				return len(onlineTracks)
			}
			return len(view)
		},
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if showingOnline {
				if i >= 0 && i < len(onlineTracks) {
					track := onlineTracks[i]
					text := fmt.Sprintf("%s - %s [%s]", track.Title, track.Artist, track.Duration)
					o.(*widget.Label).SetText(text)
				}
			} else {
				if i >= 0 && i < len(view) {
					base := filepath.Base(view[i])
					o.(*widget.Label).SetText(base)
				}
			}
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		if suppressSelect {
			return
		}
		updateInfo := func(path string) {
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			go func() {
				defer cancel()
				if info, err := meta.Lookup(ctx, base); err == nil {
					fyne.Do(func() {
						titleLbl.SetText(info.Title)
						artistLbl.SetText(info.Artist)
						albumLbl.SetText(info.Album)
						if info.Artwork != "" {
							if img, e := downloadImage(info.Artwork); e == nil && img != nil {
								cover.Image = img
								cover.Refresh()
							}
						}
						// Update Discord presence with metadata
						_ = dc.UpdatePresence(path, info.Artist, info.Title, false)
					})
				}
			}()
		}

		// Handle online track selection
		if showingOnline {
			if id < 0 || id >= len(onlineTracks) {
				return
			}
			track := onlineTracks[id]
			currentTrack.SetText(track.Title)
			titleLbl.SetText(track.Title)
			artistLbl.SetText(track.Artist)
			albumLbl.SetText("")

			// Stop any playing media
			if vplayer != nil && vplayer.IsPlaying() {
				vplayer.Stop()
			}
			if p.IsPlaying() {
				p.Stop()
			}
			visualShow(false)

			preferred := strings.ToLower(st.Settings.DownloadFormat)
			if preferred == "mp4" {
				// Prefer video flow
				if downloaded, localPath := streaming.IsDownloadedVideo(track, dbDir); downloaded {
					albumLbl.SetText("💾 Yerel Video")
					selected = localPath

					visualShow(true)
					if vplayer == nil {
						vp, err := video.New()
						if err == nil {
							vplayer = vp
							if vis := vplayer.Visual(); vis != nil {
								videoBox.Objects = []fyne.CanvasObject{vis}
								videoBox.Refresh()
							}
						}
					}
					if vplayer != nil {
						_ = vplayer.Load(selected)
						_ = vplayer.Play()
						toggleBtn.SetText("⏸")
						_ = dc.UpdatePresence(selected, track.Artist, track.Title, false)
					}
					progress.Enable()
					return
				}

				albumLbl.SetText("🔽 Video İndiriliyor...")
				go func() {
					localPath, err := streaming.DownloadVideo(track, dbDir)
					if err != nil {
						fyne.Do(func() {
							albumLbl.SetText("❌ Hata")
							dialog.ShowError(fmt.Errorf("video indirme hatası: %w", err), w)
						})
						return
					}
					if f, e := scanMusic(dbDir); e == nil {
						files = f
					}
					fyne.Do(func() {
						albumLbl.SetText("✅ Video İndirildi")
						selected = localPath
						visualShow(true)
						if vplayer == nil {
							vp, err := video.New()
							if err == nil {
								vplayer = vp
								if vis := vplayer.Visual(); vis != nil {
									videoBox.Objects = []fyne.CanvasObject{vis}
									videoBox.Refresh()
								}
							}
						}
						if vplayer != nil {
							_ = vplayer.Load(localPath)
							_ = vplayer.Play()
							toggleBtn.SetText("⏸")
							_ = dc.UpdatePresence(localPath, track.Artist, track.Title, false)
						}
						progress.Enable()
					})
				}()
				return
			}

			// Prefer audio flow (mp3)
			if downloaded, localPath := streaming.IsDownloaded(track, dbDir); downloaded {
				albumLbl.SetText("💾 Yerel")
				selected = localPath
				if err := p.Load(selected); err != nil {
					dialog.ShowError(err, w)
					return
				}
				progress.Enable()
				p.Play()
				toggleBtn.SetText("⏸")
				_ = dc.UpdatePresence(selected, track.Artist, track.Title, false)
				return
			}

			albumLbl.SetText("🔽 İndiriliyor...")
			go func() {
				localPath, err := streaming.Download(track, dbDir)
				if err != nil {
					fyne.Do(func() {
						albumLbl.SetText("❌ Hata")
						dialog.ShowError(fmt.Errorf("indirme hatası: %w", err), w)
					})
					return
				}
				if f, e := scanMusic(dbDir); e == nil {
					files = f
				}
				fyne.Do(func() {
					albumLbl.SetText("✅ İndirildi")
					selected = localPath
					if err := p.Load(localPath); err != nil {
						dialog.ShowError(err, w)
						return
					}
					progress.Enable()
					p.Play()
					toggleBtn.SetText("⏸")
					_ = dc.UpdatePresence(localPath, track.Artist, track.Title, false)
				})
			}()
			return
		}

		// Handle local file selection
		if id >= 0 && id < len(view) {
			selected = view[id]
			currentTrack.SetText(filepath.Base(selected))

			ext := strings.ToLower(filepath.Ext(selected))
			if ext == ".mp4" {
				// Stop audio player if playing
				if p.IsPlaying() {
					p.Stop()
				}

				visualShow(true)
				updateInfo(selected)

				// Initialize video player if needed
				if vplayer == nil {
					vp, err := video.New()
					if err == nil {
						vplayer = vp
						// Setup video visual once
						if vis := vplayer.Visual(); vis != nil {
							videoBox.Objects = []fyne.CanvasObject{vis}
							videoBox.Refresh()
						}
					}
				}

				if vplayer != nil {
					_ = vplayer.Load(selected)
					_ = vplayer.Play()
					toggleBtn.SetText("⏸")
				}
				progress.Enable()
				updateInfo(selected)
				return
			}

			// Audio playback
			// Stop video player if playing
			if vplayer != nil && vplayer.IsPlaying() {
				vplayer.Stop()
			}

			visualShow(false)
			if err := p.Load(selected); err != nil {
				dialog.ShowError(err, w)
				return
			}
			progress.Enable()
			p.Play()
			toggleBtn.SetText("⏸")
			updateInfo(selected)
		}
	}

	// Video gömme geçici olarak devre dışı; MP4'ler sistem oynatıcıda açılır.

	// Bottom control bar
	currentTrack = widget.NewLabel("")
	currentTrack.Wrapping = fyne.TextTruncate // Truncate long names with ...
	toggleBtn = widget.NewButton("▶", nil)

	refreshBtn := widget.NewButtonWithIcon("Yenile", theme.ViewRefreshIcon(), func() {
		f, err := scanMusic(dbDir)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		files = f
		applyView()
		list.Refresh()
	})

	posLabel = widget.NewLabel("00:00")
	durLabel = widget.NewLabel("00:00")
	progress = widget.NewSlider(0, 1)
	progress.Step = 0.001
	progress.Disable() // enable for audio when playing

	// guard to avoid feedback when we update slider programmatically
	updatingProgress := false

	// like & add to playlist
	likeBtn := widget.NewButton("❤", func() {
		if selected == "" {
			return
		}
		st.Liked[selected] = !st.Liked[selected]
		_ = state.Save("data/state.json", st)
	})
	addToPlBtn := widget.NewButton("Listeye Ekle", func() {
		if selected == "" {
			return
		}
		// Build a dialog that lists existing playlists and allows creating a new one
		var dlg dialog.Dialog
		// existing playlists buttons
		names := make([]string, 0, len(st.Playlists))
		for name := range st.Playlists {
			names = append(names, name)
		}
		sort.Strings(names)
		items := []fyne.CanvasObject{widget.NewLabel("Bir playlist seçin:")}
		for _, name := range names {
			n := name
			items = append(items, widget.NewButton(n, func() {
				st.Playlists[n] = append(st.Playlists[n], selected)
				_ = state.Save("data/state.json", st)
				refreshPlaylists()
				dlg.Hide()
			}))
		}
		items = append(items, widget.NewSeparator(), widget.NewLabel("Yeni playlist:"))
		nameEntry := widget.NewEntry()
		items = append(items, nameEntry)
		items = append(items, widget.NewButton("Oluştur ve ekle", func() {
			name := strings.TrimSpace(nameEntry.Text)
			if name == "" {
				return
			}
			if _, ok := st.Playlists[name]; !ok {
				st.Playlists[name] = []string{}
			}
			st.Playlists[name] = append(st.Playlists[name], selected)
			_ = state.Save("data/state.json", st)
			refreshPlaylists()
			dlg.Hide()
		}))
		content := container.NewVBox(items...)
		dlg = dialog.NewCustom("Playliste Ekle", "Kapat", content, w)
		dlg.Show()
	})

	// Ses seviyesi
	volSlider := widget.NewSlider(0, 1)
	volSlider.Step = 0.01
	volSlider.Value = 1
	volSlider.OnChanged = func(v float64) {
		p.SetVolume(v)
		// Also set volume for video player if it exists
		if vplayer != nil {
			_ = vplayer.SetVolume(v)
		}
	}

	// Play/Pause toggle
	toggleBtn.OnTapped = func() {
		if selected == "" {
			dialog.ShowInformation("Bilgi", "Lütfen bir parça seçin.", w)
			return
		}
		ext := strings.ToLower(filepath.Ext(selected))
		if ext == ".mp4" {
			if vplayer == nil {
				vp, err := video.New()
				if err == nil {
					vplayer = vp
				}
			}
			if vplayer != nil {
				if vplayer.Current() != selected {
					_ = vplayer.Load(selected)
				}
				if !vplayer.IsPlaying() {
					// Set initial volume
					_ = vplayer.SetVolume(volSlider.Value)
					_ = vplayer.Play()
					toggleBtn.SetText("⏸")
					_ = dc.UpdatePresence(selected, artistLbl.Text, titleLbl.Text, false)
				} else {
					_ = vplayer.Pause()
					toggleBtn.SetText("▶")
					_ = dc.UpdatePresence(selected, artistLbl.Text, titleLbl.Text, true)
				}
			}
			progress.Enable() // Enable progress for video too
			return
		}
		if !p.IsPlaying() {
			cur, _ := p.CurrentFile()
			if cur != selected {
				if err := p.Load(selected); err != nil {
					dialog.ShowError(fmt.Errorf("yüklenemedi: %w", err), w)
					return
				}
				currentTrack.SetText(filepath.Base(selected))
			}
			p.Play()
			progress.Enable()
			toggleBtn.SetText("⏸")
			_ = dc.UpdatePresence(selected, artistLbl.Text, titleLbl.Text, false)
		} else {
			p.Pause()
			toggleBtn.SetText("▶")
			_ = dc.UpdatePresence(selected, artistLbl.Text, titleLbl.Text, true)
		}
	}

	// Seek support for audio and video
	progress.OnChanged = func(v float64) {
		if updatingProgress {
			return
		}
		ext := strings.ToLower(filepath.Ext(selected))
		if ext == ".mp4" {
			if vplayer != nil {
				_ = vplayer.SetPosition(v)
			}
		} else {
			_ = p.SeekRatio(v)
		}
	}

	// Control bar with better layout: fixed-width buttons, flexible progress
	trackBox := container.NewMax(currentTrack)
	trackBox.Resize(fyne.NewSize(200, 40)) // Fixed width for track name

	progressBox := container.NewBorder(nil, nil, posLabel, durLabel, progress)

	controls := container.NewBorder(
		nil, nil,
		// Left side: track info and buttons
		container.NewHBox(trackBox, toggleBtn, likeBtn, addToPlBtn),
		// Right side: volume
		container.NewHBox(widget.NewLabel("🔊"), volSlider),
		// Center: progress bar
		progressBox,
	)

	// left navigation
	var navHeader fyne.CanvasObject
	if res := loadResource("logo.png"); res != nil {
		img := canvas.NewImageFromResource(res)
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(180, 60))
		navHeader = img
	} else {
		navHeader = widget.NewLabelWithStyle("Opentify", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	homeBtn := widget.NewButtonWithIcon("Anasayfa", theme.HomeIcon(), func() { currentPage = "Anasayfa"; applyView(); list.Refresh() })
	exploreBtn := widget.NewButtonWithIcon("Keşfet", theme.SearchIcon(), func() { currentPage = "Keşfet"; applyView(); list.Refresh() })
likedBtn := widget.NewButtonWithIcon("Beğendiklerim", theme.InfoIcon(), func() { currentPage = "Beğendiklerim"; applyView(); list.Refresh() })
	addPlBtn := widget.NewButtonWithIcon("Yeni Playlist", theme.ContentAddIcon(), func() {
		name := widget.NewEntry()
		d := dialog.NewForm("Yeni Playlist", "Oluştur", "İptal",
			[]*widget.FormItem{widget.NewFormItem("Ad", name)}, func(ok bool) {
				if !ok {
					return
				}
				n := strings.TrimSpace(name.Text)
				if n == "" {
					return
				}
				if _, ok := st.Playlists[n]; !ok {
					st.Playlists[n] = []string{}
				}
				_ = state.Save("data/state.json", st)
			}, w)
		d.Show()
	})

	plHeader := container.NewHBox(widget.NewIcon(theme.FolderIcon()), widget.NewLabel("Playlistler"))
	plBox := container.NewVBox(plHeader)
	refreshPlaylists = func() {
		children := []fyne.CanvasObject{plHeader}
		for name := range st.Playlists {
			plName := name
			children = append(children, widget.NewButtonWithIcon(plName, theme.FolderIcon(), func() { currentPage = "Playlist"; currentPlaylist = plName; applyView(); list.Refresh() }))
		}
		plBox.Objects = children
		plBox.Refresh()
	}
	refreshPlaylists()

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Ara...")

	// Declare buttons first
	var onlineSearchBtn *widget.Button
	var localSearchBtn *widget.Button

	// Online search button
	onlineSearchBtn = widget.NewButton("🌐 Çevrimiçi Ara", func() {
		query := strings.TrimSpace(searchEntry.Text)
		if query == "" {
			dialog.ShowInformation("Bilgi", "Arama terimi girin.", w)
			return
		}

		// Show loading
		onlineSearchBtn.Disable()
		onlineSearchBtn.SetText("Aranıyor...")

		go func() {
			tracks, err := streaming.SearchYouTube(query, 20)
			fyne.Do(func() {
				onlineSearchBtn.Enable()
				onlineSearchBtn.SetText("🌐 Çevrimiçi Ara")

				if err != nil {
					dialog.ShowError(fmt.Errorf("arama hatası: %w", err), w)
					return
				}

				if len(tracks) == 0 {
					dialog.ShowInformation("Bilgi", "Sonuç bulunamadı.", w)
					return
				}

				onlineTracks = tracks
				showingOnline = true
				list.Refresh()
			})
		}()
	})

	// Local search button
	localSearchBtn = widget.NewButton("💻 Yerel", func() {
		showingOnline = false
		applyView()
		list.Refresh()
	})

	searchBox := container.NewBorder(nil, nil, nil, container.NewHBox(localSearchBtn, onlineSearchBtn), searchEntry)

	// Settings page
	settingsTitle := widget.NewLabelWithStyle("Ayarlar", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	// Download format select
	dlSelect := widget.NewSelect([]string{"MP3", "MP4"}, func(val string) {
		if val == "" {
			return
		}
		st.Settings.DownloadFormat = strings.ToLower(val)
		_ = state.Save("data/state.json", st)
	})
	if strings.ToLower(st.Settings.DownloadFormat) == "mp4" {
		dlSelect.SetSelected("MP4")
	} else {
		dlSelect.SetSelected("MP3")
	}
	// Theme select
	themeSelect := widget.NewSelect([]string{"Açık", "Koyu"}, func(val string) {
		v := strings.ToLower(strings.TrimSpace(val))
		if v == "koyu" {
			a.Settings().SetTheme(theme.DarkTheme())
			st.Settings.Theme = "dark"
		} else {
			a.Settings().SetTheme(theme.LightTheme())
			st.Settings.Theme = "light"
		}
		_ = state.Save("data/state.json", st)
	})
	if strings.ToLower(st.Settings.Theme) == "dark" {
		themeSelect.SetSelected("Koyu")
	} else {
		themeSelect.SetSelected("Açık")
	}
	settingsBox := container.NewVBox(
		settingsTitle,
		widget.NewSeparator(),
		widget.NewLabel("İndirme formatı"), dlSelect,
		widget.NewSeparator(),
		widget.NewLabel("Tema"), themeSelect,
	)

	// Sayfa içerikleri: Anasayfa ve Keşfet (liste)
	homeBox := container.NewVBox(
		widget.NewLabelWithStyle("Opentify'a hoş geldiniz", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Müziğinizi keşfetmek için soldan 'Keşfet' sekmesine geçin."),
	)
	exploreArea := container.NewBorder(searchBox, nil, nil, nil, list)
	settingsBox.Hide()
	pages := container.NewStack(homeBox, exploreArea, settingsBox)

	// Sağ panel: kapak + bilgiler
	cover = canvas.NewImageFromResource(theme.FileImageIcon())
	cover.FillMode = canvas.ImageFillContain
	cover.SetMinSize(fyne.NewSize(320, 240))
	videoBox = container.NewMax()
	videoBox.Hide() // initially hidden
	visualStack := container.NewStack(cover, videoBox)
	visualStack.Resize(fyne.NewSize(320, 240))
	visualShow = func(showVideo bool) {
		if showVideo {
			videoBox.Show()
			cover.Hide()
		} else {
			videoBox.Hide()
			cover.Show()
		}
	}
	// default show cover
	visualShow(false)

	titleLbl = widget.NewLabel("")
	artistLbl = widget.NewLabel("")
	albumLbl = widget.NewLabel("")
	// Uzun metinler paneli büyütmesin diye tek satır ve '...' ile kısalt
	titleLbl.Wrapping = fyne.TextTruncate
	artistLbl.Wrapping = fyne.TextTruncate
	albumLbl.Wrapping = fyne.TextTruncate
	metaInfo := container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Başlık", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), titleLbl,
		widget.NewLabelWithStyle("Sanatçı", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), artistLbl,
		widget.NewLabelWithStyle("Albüm", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), albumLbl,
	)
	infoBox := container.NewBorder(visualStack, metaInfo, nil, nil)
	rightPanel := container.NewBorder(
		widget.NewLabelWithStyle("Şimdi Çalıyor", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		infoBox,
	)

	applyView = func() {
		// Reset online search when switching pages
		if currentPage != "Keşfet" {
			showingOnline = false
		}

		q := strings.ToLower(strings.TrimSpace(searchEntry.Text))
		switch currentPage {
		case "Anasayfa":
			// Anasayfa: metin göster, listeyi gizle
			homeBox.Show()
			exploreArea.Hide()
			settingsBox.Hide()
			view = view[:0]
			list.Refresh()
			return
		case "Ayarlar":
			homeBox.Hide()
			exploreArea.Hide()
			settingsBox.Show()
			view = view[:0]
			list.Refresh()
			return
		case "Beğendiklerim":
			homeBox.Hide()
			exploreArea.Show()
			settingsBox.Hide()
			view = view[:0]
			for _, f := range files {
				if st.Liked[f] && (q == "" || strings.Contains(strings.ToLower(filepath.Base(f)), q)) {
					view = append(view, f)
				}
			}
		case "Playlist":
			homeBox.Hide()
			exploreArea.Show()
			settingsBox.Hide()
			view = view[:0]
			for _, f := range st.Playlists[currentPlaylist] {
				if q == "" || strings.Contains(strings.ToLower(filepath.Base(f)), q) {
					view = append(view, f)
				}
			}
		default: // Keşfet
			homeBox.Hide()
			exploreArea.Show()
			settingsBox.Hide()
			// Don't filter online results
			if showingOnline {
				return
			}
			view = view[:0]
			for _, f := range files {
				if q == "" || strings.Contains(strings.ToLower(filepath.Base(f)), q) {
					view = append(view, f)
				}
			}
		}
		// Try to keep selection if it still exists in current view
		if selected != "" {
			for i, f := range view {
				if f == selected {
					suppressSelect = true
					list.Select(i)
					suppressSelect = false
					break
				}
			}
		}
	}

	searchEntry.OnChanged = func(s string) {
		// Only apply filter for local files, not online
		if !showingOnline {
			applyView()
			list.Refresh()
		}
	}

	settingsBtn := widget.NewButtonWithIcon("Ayarlar", theme.SettingsIcon(), func() { currentPage = "Ayarlar"; applyView(); list.Refresh() })
	left := container.NewVBox(
		navHeader,
		widget.NewSeparator(),
		homeBtn,
		exploreBtn,
		likedBtn,
		settingsBtn,
		widget.NewSeparator(),
		addPlBtn,
		plBox,
		widget.NewSeparator(),
		refreshBtn,
	)

	mid := container.NewHSplit(pages, rightPanel)
	mid.Offset = 0.68
	split := container.NewHSplit(left, mid)
	split.Offset = 0.22
	content := container.NewBorder(nil, controls, nil, nil, split)
	w.SetContent(content)

	applyView()

	// UI ticker to update audio and video progress
	ticker := time.NewTicker(200 * time.Millisecond)
	go func() {
		for range ticker.C {
			// Check if audio is playing
			if p.IsPlaying() {
				pos, e1 := p.Position()
				dur, e2 := p.Duration()
				if e1 == nil && e2 == nil && dur > 0 {
					pr := clamp01(float64(pos) / float64(dur))
					fyne.Do(func() {
						updatingProgress = true
						posLabel.SetText(formatDur(pos))
						durLabel.SetText(formatDur(dur))
						progress.SetValue(pr)
						updatingProgress = false
					})
				}
				continue
			}
			// Check if video is playing
			if vplayer != nil && vplayer.IsPlaying() {
				pos, e1 := vplayer.Position()
				dur, e2 := vplayer.Duration()
				if e1 == nil && e2 == nil && dur > 0 {
					pr := clamp01(float64(pos) / float64(dur))
					fyne.Do(func() {
						updatingProgress = true
						posLabel.SetText(formatDur(pos))
						durLabel.SetText(formatDur(dur))
						progress.SetValue(pr)
						updatingProgress = false
					})
				}
			}
		}
	}()

	w.ShowAndRun()
}

func formatDur(d time.Duration) string {
	if d < 0 {
		return "00:00"
	}
	s := int(d.Seconds())
	m := s / 60
	ss := s % 60
	return fmt.Sprintf("%02d:%02d", m, ss)
}

func downloadImage(u string) (image.Image, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
