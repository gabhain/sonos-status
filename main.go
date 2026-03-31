package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type RoomUI struct {
	Container    *fyne.Container
	BgRect       *canvas.Rectangle
	LastArtURL   string
	NameLabel    *widget.Label
	ModelLabel   *widget.Label
	IPLabel      *widget.Label
	TrackLabel   *widget.Label
	ArtistLabel  *widget.Label
	FormatLabel  *canvas.Text
	AlbumArt     *canvas.Image
	ArtContainer *fyne.Container
	PlayPauseBtn *widget.Button
	PrevBtn      *widget.Button
	NextBtn      *widget.Button
	MuteBtn      *widget.Button
	NightBtn     *widget.Button
	SpeechBtn    *widget.Button
	LoudnessBtn  *widget.Button
	VolSlider    *widget.Slider
	BassSlider   *widget.Slider
	TrebleSlider *widget.Slider
	ProgressBar  *widget.ProgressBar
	WifiLabel    *widget.Label
	TimeLabel    *widget.Label
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Sonos Status Utility")
	myWindow.Resize(fyne.NewSize(650, 850))

	// System Tray Setup
	if desk, ok := myApp.(desktop.App); ok {
		m := fyne.NewMenu("Sonos Status",
			fyne.NewMenuItem("Show Dashboard", func() {
				myWindow.Show()
			}),
		)
		desk.SetSystemTrayMenu(m)
	}

	// Close to tray
	myWindow.SetCloseIntercept(func() {
		myWindow.Hide()
	})

	var speakers []*Speaker
	roomUIs := make(map[string]*RoomUI)
	accordion := widget.NewAccordion()

	statusLabel := widget.NewLabel("Ready")
	var eventServer *EventServer

	refreshFunc := func() {
		fyne.Do(func() { statusLabel.SetText("Scanning...") })
		go func() {
			newSpeakers, err := DiscoverSpeakers()
			fyne.Do(func() {
				if err != nil {
					statusLabel.SetText(fmt.Sprintf("Error: %v", err))
					return
				}
				speakers = newSpeakers
				statusLabel.SetText(fmt.Sprintf("Found %d rooms.", len(speakers)))
				
				accordion.Items = nil
				for _, s := range speakers {
					ui := createRoomUI(s)
					roomUIs[s.UID] = ui
					
					// Set callback for EventServer
					s.OnUpdate = func() {
						fyne.Do(func() {
							updateUI(ui, s)
							accordion.Refresh()
						})
					}
					
					item := widget.NewAccordionItem(s.Name, ui.Container)
					accordion.Append(item)
				}
				if len(accordion.Items) > 0 {
					accordion.Open(0)
				}
				accordion.Refresh()

				// Start Event Server
				if eventServer != nil {
					// Stop old one if needed? (For simplicity, we'll just create one)
				}
				es, err := NewEventServer(speakers)
				if err == nil {
					eventServer = es
					go eventServer.Start()
				}
			})
		}()
	}

	go func() {
		// Slow down the ticker - it's now just a safety fallback 
		// for track progress which doesn't always send events every second
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			for _, s := range speakers {
				s.UpdateStatus()
				fyne.Do(func() {
					if ui, ok := roomUIs[s.UID]; ok {
						updateUI(ui, s)
						for _, item := range accordion.Items {
							if item.Detail == ui.Container {
								item.Title = s.Name
								break
							}
						}
					}
				})
			}
			fyne.Do(func() { accordion.Refresh() })
		}
	}()

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), refreshFunc)
	topBar := container.NewBorder(nil, nil, nil, refreshBtn, statusLabel)
	scroll := container.NewVScroll(accordion)
	mainContent := container.NewBorder(topBar, nil, nil, nil, scroll)
	myWindow.SetContent(mainContent)

	go func() {
		time.Sleep(200 * time.Millisecond)
		refreshFunc()
	}()

	myWindow.ShowAndRun()
}

func createRoomUI(s *Speaker) *RoomUI {
	ui := &RoomUI{}
	
	// 1. Playback Info (Art + Labels)
	ui.TrackLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ui.TrackLabel.Wrapping = fyne.TextWrapWord
	ui.ArtistLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	ui.ArtistLabel.Wrapping = fyne.TextWrapWord
	
	ui.FormatLabel = canvas.NewText("", color.NRGBA{R: 100, G: 200, B: 255, A: 255})
	ui.FormatLabel.TextSize = 13
	ui.FormatLabel.TextStyle.Bold = true

	ui.AlbumArt = canvas.NewImageFromResource(theme.BrokenImageIcon())
	ui.AlbumArt.FillMode = canvas.ImageFillContain
	ui.AlbumArt.SetMinSize(fyne.NewSize(100, 100))
	ui.ArtContainer = container.NewStack(ui.AlbumArt)
	
	ui.ProgressBar = widget.NewProgressBar()
	ui.TimeLabel = widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true})
	
	playbackInfo := container.NewVBox(ui.TrackLabel, ui.ArtistLabel, container.NewHBox(ui.FormatLabel, layout.NewSpacer(), ui.TimeLabel), ui.ProgressBar)
	playbackRow := container.NewBorder(nil, nil, container.New(layout.NewGridWrapLayout(fyne.NewSize(100, 100)), ui.ArtContainer), nil, playbackInfo)

	// 2. Main Controls (Play/Pause, Mute, Volume)
	ui.PlayPauseBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		go func() {
			s.TogglePlay()
			s.UpdateStatus()
			fyne.Do(func() { updateUI(ui, s) })
		}()
	})
	ui.PrevBtn = widget.NewButtonWithIcon("", theme.MediaSkipPreviousIcon(), func() {
		go func() {
			s.Previous()
			s.UpdateStatus()
			fyne.Do(func() { updateUI(ui, s) })
		}()
	})
	ui.NextBtn = widget.NewButtonWithIcon("", theme.MediaSkipNextIcon(), func() {
		go func() {
			s.Next()
			s.UpdateStatus()
			fyne.Do(func() { updateUI(ui, s) })
		}()
	})
	
	ui.MuteBtn = widget.NewButtonWithIcon("", theme.VolumeUpIcon(), func() {
		go func() {
			s.ToggleMute()
			s.UpdateStatus()
			fyne.Do(func() { updateUI(ui, s) })
		}()
	})

	ui.VolSlider = widget.NewSlider(0, 100)
	ui.VolSlider.OnChanged = func(v float64) {
		go s.SetVolume(uint16(v))
	}
	
	controlsRow := container.NewBorder(nil, nil, container.NewHBox(ui.PrevBtn, ui.PlayPauseBtn, ui.NextBtn, ui.MuteBtn), nil, ui.VolSlider)

	// 3. Audio Modes & EQ (Night, Speech, Loudness + Bass/Treble)
	ui.NightBtn = widget.NewButtonWithIcon("Night", theme.VisibilityIcon(), func() {
		go func() {
			s.ToggleNightMode()
			s.UpdateStatus()
			fyne.Do(func() { updateUI(ui, s) })
		}()
	})
	ui.SpeechBtn = widget.NewButtonWithIcon("Speech", theme.VolumeUpIcon(), func() {
		go func() {
			s.ToggleSpeechEnhancement()
			s.UpdateStatus()
			fyne.Do(func() { updateUI(ui, s) })
		}()
	})
	ui.LoudnessBtn = widget.NewButtonWithIcon("Loudness", theme.VolumeUpIcon(), func() {
		go func() {
			s.ToggleLoudness()
			s.UpdateStatus()
			fyne.Do(func() { updateUI(ui, s) })
		}()
	})
	
	ui.BassSlider = widget.NewSlider(-10, 10)
	ui.BassSlider.OnChanged = func(v float64) {
		go s.SetBass(int(v))
	}
	ui.TrebleSlider = widget.NewSlider(-10, 10)
	ui.TrebleSlider.OnChanged = func(v float64) {
		go s.SetTreble(int(v))
	}

	eqRow := container.NewGridWithColumns(2, 
		container.NewVBox(widget.NewLabelWithStyle("Bass", fyne.TextAlignCenter, fyne.TextStyle{Italic: true}), ui.BassSlider),
		container.NewVBox(widget.NewLabelWithStyle("Treble", fyne.TextAlignCenter, fyne.TextStyle{Italic: true}), ui.TrebleSlider),
	)

	modesRow := container.NewVBox(
		container.NewGridWithColumns(3, ui.NightBtn, ui.SpeechBtn, ui.LoudnessBtn),
		eqRow,
	)

	// 4. Room Info (Hardware, IP, Wi-Fi)
	ui.ModelLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	ui.ModelLabel.Wrapping = fyne.TextWrapWord
	ui.IPLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	ui.WifiLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true, Bold: true})
	
	hwTitle := widget.NewLabelWithStyle("Hardware:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	
	// Use a Border to keep Hardware title and Model list on the same line
	hwRow := container.NewBorder(nil, nil, hwTitle, nil, ui.ModelLabel)

	infoRow := container.NewVBox(
		widget.NewSeparator(),
		hwRow,
		ui.IPLabel,
		ui.WifiLabel,
	)

	// Combine into card-like structure
	ui.BgRect = canvas.NewRectangle(color.NRGBA{R: 30, G: 30, B: 30, A: 255})
	ui.Container = container.NewMax(ui.BgRect, container.NewPadded(container.NewVBox(
		playbackRow,
		container.NewPadded(controlsRow),
		modesRow,
		infoRow,
	)))

	updateUI(ui, s)
	return ui
}

func getAverageColor(res fyne.Resource) color.Color {
	img, _, err := image.Decode(bytes.NewReader(res.Content()))
	if err != nil {
		return color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	}

	var r, g, b, count uint64
	bounds := img.Bounds()
	// Sample every 5th pixel for speed
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 5 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 5 {
			pr, pg, pb, _ := img.At(x, y).RGBA()
			r += uint64(pr >> 8)
			g += uint64(pg >> 8)
			b += uint64(pb >> 8)
			count++
		}
	}
	if count == 0 {
		return color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	}

	// Calculate average and blend 30/70 with dark theme background (30,30,30)
	avgR := uint8(r / count)
	avgG := uint8(g / count)
	avgB := uint8(b / count)
	
	tintR := uint8(float64(avgR)*0.2 + 30*0.8)
	tintG := uint8(float64(avgG)*0.2 + 30*0.8)
	tintB := uint8(float64(avgB)*0.2 + 30*0.8)
	
	return color.NRGBA{R: tintR, G: tintG, B: tintB, A: 255}
}

func updateUI(ui *RoomUI, s *Speaker) {
	// Track Info
	ui.TrackLabel.SetText(s.Track)
	if s.Artist != "" {
		ui.ArtistLabel.SetText(s.Artist)
		ui.ArtistLabel.Show()
	} else {
		ui.ArtistLabel.Hide()
	}

	// Format
	if s.AudioFormat != "" {
		ui.FormatLabel.Text = s.AudioFormat
		ui.FormatLabel.Show()
	} else {
		ui.FormatLabel.Hide()
	}

	// Progress
	if s.DurationSec > 0 {
		ui.ProgressBar.SetValue(float64(s.ProgressSec) / float64(s.DurationSec))
		ui.ProgressBar.Show()
		ui.TimeLabel.SetText(fmt.Sprintf("%s / %s", s.Progress, s.Duration))
		ui.TimeLabel.Show()
	} else {
		ui.ProgressBar.Hide()
		ui.TimeLabel.Hide()
	}

	// Album Art (with Caching & Dynamic Theming)
	if s.AlbumArtURL != "" {
		if s.AlbumArtURL != ui.LastArtURL {
			res, err := fyne.LoadResourceFromURLString(s.AlbumArtURL)
			if err == nil {
				ui.AlbumArt.Resource = res
				ui.AlbumArt.Show()
				ui.ArtContainer.Show()
				ui.LastArtURL = s.AlbumArtURL
				
				// Dynamic Color extraction
				go func() {
					tintColor := getAverageColor(res)
					fyne.Do(func() {
						ui.BgRect.FillColor = tintColor
						ui.BgRect.Refresh()
					})
				}()
			} else {
				ui.AlbumArt.Hide()
				ui.ArtContainer.Hide()
				ui.BgRect.FillColor = color.NRGBA{R: 30, G: 30, B: 30, A: 255}
			}
		}
	} else {
		ui.AlbumArt.Hide()
		ui.ArtContainer.Hide()
		ui.LastArtURL = ""
		ui.BgRect.FillColor = color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	}
	ui.AlbumArt.Refresh()
	ui.BgRect.Refresh()

	// Play/Pause
	if s.IsPlaying {
		ui.PlayPauseBtn.SetIcon(theme.MediaPauseIcon())
	} else {
		ui.PlayPauseBtn.SetIcon(theme.MediaPlayIcon())
	}

	// Mute
	if s.Mute {
		ui.MuteBtn.SetIcon(theme.VolumeMuteIcon())
		ui.MuteBtn.Importance = widget.HighImportance
	} else {
		ui.MuteBtn.SetIcon(theme.VolumeUpIcon())
		ui.MuteBtn.Importance = widget.MediumImportance
	}

	// Night/Speech/Loudness Modes
	if s.NightMode {
		ui.NightBtn.Importance = widget.HighImportance
	} else {
		ui.NightBtn.Importance = widget.MediumImportance
	}
	if s.SpeechEnhancement {
		ui.SpeechBtn.Importance = widget.HighImportance
	} else {
		ui.SpeechBtn.Importance = widget.MediumImportance
	}
	if s.Loudness {
		ui.LoudnessBtn.Importance = widget.HighImportance
	} else {
		ui.LoudnessBtn.Importance = widget.MediumImportance
	}

	// Volume & EQ
	ui.VolSlider.SetValue(float64(s.Volume))
	ui.BassSlider.SetValue(float64(s.Bass))
	ui.TrebleSlider.SetValue(float64(s.Treble))

	// Wi-Fi
	ui.IPLabel.SetText(fmt.Sprintf("IP: %s", s.IP))
	if s.WifiStrength != 0 {
		percent := 0
		if s.WifiStrength == 999 {
			ui.WifiLabel.SetText("Interface: Wireless (Active)")
		} else if s.WifiStrength > 0 {
			percent = s.WifiStrength
			if percent > 100 { percent = 100 }
			ui.WifiLabel.SetText(fmt.Sprintf("Interface: Wireless (%d%% SNR)", percent))
		} else {
			if s.WifiStrength > -30 {
				percent = 100
			} else if s.WifiStrength < -90 {
				percent = 0
			} else {
				percent = int(100 * float64(s.WifiStrength-(-90)) / float64(-30-(-90)))
			}
			ui.WifiLabel.SetText(fmt.Sprintf("Interface: Wireless (%d%% RSSI)", percent))
		}
	} else {
		ui.WifiLabel.SetText("Interface: Wired")
	}
	ui.WifiLabel.Show()

	// Hardware List
	counts := make(map[string]int)
	var order []string
	for _, m := range s.Models {
		if counts[m] == 0 { order = append(order, m) }
		counts[m]++
	}
	var modelParts []string
	for _, m := range order {
		if counts[m] > 1 {
			modelParts = append(modelParts, fmt.Sprintf("%s x%d", m, counts[m]))
		} else {
			modelParts = append(modelParts, m)
		}
	}
	ui.ModelLabel.SetText(strings.Join(modelParts, " + "))
}
