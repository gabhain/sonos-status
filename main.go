package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type RoomUI struct {
	Container    *fyne.Container
	NameLabel    *widget.Label
	ModelLabel   *widget.Label
	IPLabel      *widget.Label
	TrackLabel   *widget.Label
	ArtistLabel  *widget.Label
	FormatLabel  *canvas.Text
	AlbumArt     *canvas.Image
	ArtContainer *fyne.Container
	PlayPauseBtn *widget.Button
	MuteBtn      *widget.Button
	NightBtn     *widget.Button
	SpeechBtn    *widget.Button
	LoudnessBtn  *widget.Button
	VolSlider    *widget.Slider
	ProgressBar  *widget.ProgressBar
	WifiLabel    *widget.Label
	TimeLabel    *widget.Label
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Sonos Status Utility")
	myWindow.Resize(fyne.NewSize(650, 850))

	var speakers []*Speaker
	roomUIs := make(map[string]*RoomUI)
	accordion := widget.NewAccordion()

	statusLabel := widget.NewLabel("Ready")

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
					
					item := widget.NewAccordionItem(s.Name, ui.Container)
					accordion.Append(item)
				}
				if len(accordion.Items) > 0 {
					accordion.Open(0)
				}
				accordion.Refresh()
			})
		}()
	}

	go func() {
		ticker := time.NewTicker(3 * time.Second)
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
	
	controlsRow := container.NewBorder(nil, nil, container.NewHBox(ui.PlayPauseBtn, ui.MuteBtn), nil, ui.VolSlider)

	// 3. Audio Modes (Night, Speech, Loudness)
	ui.NightBtn = widget.NewButtonWithIcon("Night Mode", theme.VisibilityIcon(), func() {
		go func() {
			s.ToggleNightMode()
			s.UpdateStatus()
			fyne.Do(func() { updateUI(ui, s) })
		}()
	})
	ui.SpeechBtn = widget.NewButtonWithIcon("Speech Enhancement", theme.VolumeUpIcon(), func() {
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
	modesRow := container.NewGridWithColumns(3, ui.NightBtn, ui.SpeechBtn, ui.LoudnessBtn)

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
	bg := canvas.NewRectangle(color.NRGBA{R: 30, G: 30, B: 30, A: 255})
	ui.Container = container.NewMax(bg, container.NewPadded(container.NewVBox(
		playbackRow,
		container.NewPadded(controlsRow),
		modesRow,
		infoRow,
	)))

	updateUI(ui, s)
	return ui
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

	// Album Art
	if s.AlbumArtURL != "" {
		res, err := fyne.LoadResourceFromURLString(s.AlbumArtURL)
		if err == nil {
			ui.AlbumArt.Resource = res
			ui.AlbumArt.Show()
			ui.ArtContainer.Show()
		} else {
			ui.AlbumArt.Hide()
			ui.ArtContainer.Hide()
		}
	} else {
		ui.AlbumArt.Hide()
		ui.ArtContainer.Hide()
	}
	ui.AlbumArt.Refresh()

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

	// Volume
	ui.VolSlider.SetValue(float64(s.Volume))

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
