package main

import (
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	sonos "github.com/HandyGold75/Gonos"
)

type Speaker struct {
	Name              string
	UID               string
	Model             string   // Main model (the one used for control)
	Models            []string // All hardware in the room
	IP                string
	Volume            uint16
	Mute              bool
	Track             string
	Artist            string
	AlbumArtURL       string
	AudioFormat       string
	IsPlaying         bool
	NightMode         bool
	SpeechEnhancement bool
	Loudness          bool
	Duration          string // HH:MM:SS
	Progress          string // HH:MM:SS
	DurationSec       int
	ProgressSec       int
	WifiStrength      int // RSSI (dBm)
	Bass              int
	Treble            int
	OnUpdate          func()
	Player            *sonos.ZonePlayer
}

// FetchModelName grabs the friendly model name from the device description XML
func fetchModelName(ip string) string {
	url := fmt.Sprintf("http://%s:1400/xml/device_description.xml", ip)
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "Sonos"
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Sonos"
	}
	content := string(body)
	start := strings.Index(content, "<modelName>")
	end := strings.Index(content, "</modelName>")
	if start != -1 && end != -1 && end > start {
		return content[start+11 : end]
	}
	return "Sonos"
}

func fetchRoomName(ip string) string {
	url := fmt.Sprintf("http://%s:1400/xml/device_description.xml", ip)
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	content := string(body)
	
	roomName := ""
	start := strings.Index(content, "<roomName>")
	end := strings.Index(content, "</roomName>")
	if start != -1 && end != -1 && end > start {
		roomName = content[start+10 : end]
	}

	// If it's a Sub, the roomName might just be "Sub".
	// We'll clean it later, but if it's literally "Sub", we might need more info.
	// However, usually it's "Room Name (+Sub)".
	return roomName
}

func fetchUUID(ip string) string {
	url := fmt.Sprintf("http://%s:1400/xml/device_description.xml", ip)
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	content := string(body)
	start := strings.Index(content, "<UDN>uuid:")
	end := strings.Index(content, "</UDN>")
	if start != -1 && end != -1 && end > start {
		return content[start+10 : end]
	}
	return ""
}

func (s *Speaker) GetChannelMap() map[string]string {
	state, err := s.Player.ZoneGroupTopology.GetZoneGroupState()
	if err != nil { return nil }
	state = html.UnescapeString(state)
	
	channelMap := make(map[string]string)
	
	// Look for HTSatChanMapSet="..." in the coordinator tag
	attr := "HTSatChanMapSet=\""
	idx := strings.Index(state, attr)
	if idx == -1 { return nil }
	
	val := state[idx+len(attr):]
	end := strings.Index(val, "\"")
	if end == -1 { return nil }
	
	// Format: UUID:CH,CH;UUID:CH;...
	pairs := strings.Split(val[:end], ";")
	for _, p := range pairs {
		parts := strings.Split(p, ":")
		if len(parts) == 2 {
			uuid := parts[0]
			role := parts[1]
			// Map codes to friendly names
			friendly := role
			switch role {
			case "LF": friendly = "L"
			case "RF": friendly = "R"
			case "LR", "LS": friendly = "LS"
			case "RR", "RS": friendly = "RS"
			case "SW": friendly = "Sub"
			}
			channelMap[uuid] = friendly
		}
	}
	return channelMap
}

func (s *Speaker) GetRoomInfoFromTopology() (string, string, bool) {
	state, err := s.Player.ZoneGroupTopology.GetZoneGroupState()
	if err != nil { return "", "", false }
	
	// Sonos encodes the internal XML in the SOAP response (e.g. &lt; instead of <)
	// We MUST unescape it to parse it correctly.
	state = html.UnescapeString(state)
	
	// 1. Find our entry in the topology
	idx := strings.Index(state, s.IP)
	if idx == -1 { return "", "", false }

	// 2. Check if we are invisible (bonded)
	tagStart := strings.LastIndex(state[:idx], "<")
	tagEnd := strings.Index(state[idx:], ">")
	isInvisible := false
	if tagStart != -1 && tagEnd != -1 {
		tagContent := state[tagStart : idx+tagEnd+1]
		isInvisible = strings.Contains(tagContent, "Invisible=\"1\"")
	}

	// 3. Find the parent <ZoneGroup> to get the Coordinator UUID and Group ID
	zgIdx := strings.LastIndex(state[:idx], "<ZoneGroup ")
	if zgIdx == -1 { return "", "", isInvisible }
	
	zgTagEnd := strings.Index(state[zgIdx:], ">")
	if zgTagEnd == -1 { return "", "", isInvisible }
	zgTag := state[zgIdx : zgIdx+zgTagEnd+1]

	groupID := ""
	coordUUID := ""
	if iIdx := strings.Index(zgTag, "ID=\""); iIdx != -1 {
		val := zgTag[iIdx+4:]
		if end := strings.Index(val, "\""); end != -1 { groupID = val[:end] }
	}
	if cIdx := strings.Index(zgTag, "Coordinator=\""); cIdx != -1 {
		val := zgTag[cIdx+13:]
		if end := strings.Index(val, "\""); end != -1 { coordUUID = val[:end] }
	}

	// 4. Find the Coordinator's ZoneName
	roomName := ""
	if coordUUID != "" {
		cSearch := fmt.Sprintf("UUID=\"%s\"", coordUUID)
		cIdx := strings.Index(state, cSearch)
		if cIdx != -1 {
			// Find ZoneName in this coordinator's tag
			cTagStart := strings.LastIndex(state[:cIdx], "<")
			cTagEnd := strings.Index(state[cIdx:], ">")
			if cTagStart != -1 && cTagEnd != -1 {
				cTag := state[cTagStart : cIdx+cTagEnd+1]
				if zIdx := strings.Index(cTag, "ZoneName=\""); zIdx != -1 {
					val := cTag[zIdx+10:]
					if end := strings.Index(val, "\""); end != -1 { roomName = val[:end] }
				}
			}
		}
	}
	
	return groupID, roomName, isInvisible
}

func cleanRoomName(name string) string {
	name = strings.TrimSpace(name)
	// Remove common Sonos suffixes like (+Sub), (+LS), etc.
	// We look for a space followed by a parenthesis
	if idx := strings.Index(name, " ("); idx != -1 {
		name = name[:idx]
	}
	// Also check for (+Sub) without a space
	if idx := strings.Index(name, "(+"); idx != -1 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}

func mapHTAudioFormat(code int) string {
	switch code {
	case 0, 21:
		return ""
	case 1:
		return "Unsupported"
	case 2, 33554434:
		return "Stereo"
	case 7:
		return "Dolby Digital 2.0"
	case 13, 14, 15, 16, 59, 63:
		return "Dolby Atmos"
	case 18, 84934713:
		return "Dolby Digital 5.1"
	case 19, 84934714:
		return "Dolby Digital Plus"
	case 17, 20:
		return "DTS"
	case 22:
		return "Silence"
	default:
		if code > 0 {
			return fmt.Sprintf("Unknown Format (%d)", code)
		}
		return ""
	}
}

func DiscoverSpeakers() ([]*Speaker, error) {
	log.Printf("Starting discovery...")
	players, _ := sonos.DiscoverZonePlayer(4 * time.Second)

	knownIPs := []string{"10.0.0.33", "10.0.0.97", "10.0.0.177", "10.0.0.185", "10.0.0.191", "10.0.0.242"}
	
	var wg sync.WaitGroup
	var mu sync.Mutex
	allPlayers := make(map[string]*sonos.ZonePlayer)
	
	for _, p := range players {
		allPlayers[p.URL] = p
	}

	for _, ip := range knownIPs {
		wg.Add(1)
		go func(ipAddr string) {
			defer wg.Done()
			p, err := sonos.NewZonePlayer(ipAddr)
			if err == nil && p != nil {
				mu.Lock()
				allPlayers[p.URL] = p
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()

	var speakers []*Speaker
	roomMap := make(map[string]*Speaker)

	for _, p := range allPlayers {
		model := fetchModelName(p.ZoneInfo.IPAddress)
		uuid := fetchUUID(p.ZoneInfo.IPAddress)
		s := &Speaker{
			UID:    uuid,
			IP:     p.ZoneInfo.IPAddress,
			Model:  model,
			Models: []string{model},
			Player: p,
		}
		if s.UID == "" { s.UID = p.ZoneInfo.SerialNumber }
		
		groupID, groupName, isInvisible := s.GetRoomInfoFromTopology()
		
		// Get channel map to label satellites
		channels := s.GetChannelMap()
		if role, ok := channels[s.UID]; ok {
			s.Model = fmt.Sprintf("%s (%s)", model, role)
			s.Models = []string{s.Model}
		}

		// Fallback for names if topology search failed
		if groupName == "" || strings.ToLower(groupName) == "sub" {
			groupName = fetchRoomName(s.IP)
		}
		if groupName == "" || strings.ToLower(groupName) == "sub" {
			groupName, _ = p.GetZoneName()
		}
		if groupName == "" {
			groupName = "Sonos Speaker"
		}
		
		// If topology failed to give us a unique groupID, fallback to raw name
		if groupID == "" {
			groupID = groupName
		}
		
		s.Name = cleanRoomName(groupName)
		s.UpdateStatus()

		if existing, ok := roomMap[groupID]; ok {
			isBetter := false
			// If we are 'invisible', we NEVER become the primary speaker
			if !isInvisible {
				// Priority 1: Has TV Audio format
				if s.AudioFormat != "" && existing.AudioFormat == "" {
					isBetter = true
				} else if s.AudioFormat == "" && existing.AudioFormat != "" {
					isBetter = false
				} else if s.IsPlaying && !existing.IsPlaying {
					// Priority 2: Is playing music
					isBetter = true
				} else if strings.Contains(strings.ToLower(model), "arc") || 
						strings.Contains(strings.ToLower(model), "beam") || 
						strings.Contains(strings.ToLower(model), "playbar") ||
						strings.Contains(strings.ToLower(model), "playbase") ||
						strings.Contains(strings.ToLower(model), "amp") {
					// Priority 3: Is a main HT component
					if !strings.Contains(strings.ToLower(existing.Model), "arc") &&
					!strings.Contains(strings.ToLower(existing.Model), "beam") {
						isBetter = true
					}
				}
			}

			// Add hardware to existing room's Models list
			// Since s is a unique physical player from allPlayers, we just add its model
			existing.Models = append(existing.Models, s.Model)

			if isBetter {
				// The new 's' is better as a primary controller
				s.Models = existing.Models
				roomMap[groupID] = s
			}
		} else {
			roomMap[groupID] = s
		}
	}

	for _, s := range roomMap {
		speakers = append(speakers, s)
	}

	// Final grouping pass by Room Name (Visibility-First)
	// This mirrors the logic in professional tools like sonoscli.
	finalMap := make(map[string]*Speaker)
	for _, s := range speakers {
		name := s.Name
		if existing, ok := finalMap[name]; ok {
			// Merge models list
			existing.Models = append(existing.Models, s.Models...)

			// PRIORITY LOGIC:
			// 1. Prefer Visible rooms over Invisible (Subs/Satellites)
			// 2. Prefer Coordinator/Main models (Arc/Beam) over others
			
			// If current is visible and existing is invisible, SWAP
			_, _, curInv := s.GetRoomInfoFromTopology()
			_, _, exInv := existing.GetRoomInfoFromTopology()

			shouldReplace := false
			if !curInv && exInv {
				shouldReplace = true
			} else if curInv == exInv {
				// Both same visibility, check for main hardware
				if s.AudioFormat != "" && existing.AudioFormat == "" {
					shouldReplace = true
				}
			}

			if shouldReplace {
				s.Models = existing.Models
				finalMap[name] = s
			}
		} else {
			finalMap[name] = s
		}
	}

	speakers = nil
	for _, s := range finalMap {
		speakers = append(speakers, s)
	}

	return speakers, nil
}

func parseDuration(d string) int {
	parts := strings.Split(d, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	s, _ := strconv.Atoi(parts[2])
	return h*3600+m*60+s
}

func (s *Speaker) fetchRSSI() int {
	// Expanded paths including ifconfig and the support/review entry point
	paths := []string{
		"/status/proc/ath_rincon/status",
		"/status/proc/ath_nexus/status",
		"/status/proc/wlan0/status",
		"/status/proc/ath0/status",
		"/status/proc/mw_rincon/status",
		"/status/ifconfig",
		"/status/wireless",
	}
	client := http.Client{Timeout: 800 * time.Millisecond}
	
	for _, path := range paths {
		url := fmt.Sprintf("http://%s:1400%s", s.IP, path)
		resp, err := client.Get(url)
		if err != nil { continue }
		defer resp.Body.Close()
		
		body, err := io.ReadAll(resp.Body)
		if err != nil { continue }
		content := string(body)
		contentLower := strings.ToLower(content)
		
		// 1. High Priority: Look for "FROM XX" in node list (SonosNet/Satellite specific)
		// Usually looks like: "Node XX:XX:XX... - FROM 68 : TO 67"
		fromIdx := strings.Index(content, " - FROM ")
		if fromIdx != -1 {
			valPart := content[fromIdx+8:]
			end := strings.Index(valPart, " ")
			if end != -1 {
				val, err := strconv.Atoi(valPart[:end])
				if err == nil && val > 0 {
					return val // This is a 0-100 quality score
				}
			}
		}

		// 2. Standard keywords
		targets := []string{"snr:", "signal strength:", "rssi:", "signal:", "quality:", "strength:"}
		foundVal := 0
		found := false
		
		for _, target := range targets {
			idx := strings.Index(contentLower, target)
			if idx != -1 {
				valPart := content[idx+len(target):]
				// Find first digit or minus sign
				start := -1
				for i, r := range valPart {
					if (r >= '0' && r <= '9') || r == '-' {
						start = i
						break
					}
				}
				if start != -1 {
					end := start
					for i, r := range valPart[start:] {
						if !((r >= '0' && r <= '9') || r == '-' || r == '.') {
							end = start + i
							break
						}
					}
					if end > start {
						f, err := strconv.ParseFloat(valPart[start:end], 64)
						if err == nil && f != 0 {
							foundVal = int(f)
							found = true
							break
						}
					}
				}
			}
		}
		
		if found {
			return foundVal
		}
		
		// 3. Fallback for ifconfig
		if path == "/status/ifconfig" {
			if strings.Contains(contentLower, "ath") || strings.Contains(contentLower, "wlan") || strings.Contains(contentLower, "ra0") {
				return 999 
			}
		}
	}
	return 0
}

func (s *Speaker) UpdateStatus() error {
	if s.Player == nil {
		return nil
	}

	info, err := s.Player.GetZoneInfo()
	if err == nil {
		s.AudioFormat = mapHTAudioFormat(info.HTAudioIn)
		s.IP = info.IPAddress
	}

	state, err := s.Player.GetCurrentTransportState()
	if err == nil {
		s.IsPlaying = (state == "PLAYING")
	}

	vol, _ := s.Player.GetVolume()
	s.Volume = uint16(vol)
	mute, _ := s.Player.GetMute()
	s.Mute = mute

	// Fetch Night Mode and Speech Enhancement
	nm, err := s.Player.RenderingControl.GetEQ("NightMode")
	if err == nil {
		s.NightMode = (nm == "1")
	}
	se, err := s.Player.RenderingControl.GetEQ("DialogLevel")
	if err == nil {
		s.SpeechEnhancement = (se == "1")
	}

	// Fetch Loudness
	l, err := s.Player.GetLoudness()
	if err == nil {
		s.Loudness = l
	}

	// Fetch Bass/Treble
	b, _ := s.Player.GetBass()
	s.Bass = b
	t, _ := s.Player.GetTreble()
	s.Treble = t

	// Fetch Wi-Fi Strength (Scrape status page)
	s.WifiStrength = s.fetchRSSI()

	track, err := s.Player.GetTrackInfo()
	if err == nil && track.Title != "" && !strings.Contains(track.URI, "htastream") && !strings.Contains(track.URI, "x-rincon:") {
		s.Track = cleanString(track.Title)
		s.Artist = cleanString(track.Creator)
		s.Duration = track.Duration
		s.Progress = track.Progress
		s.DurationSec = parseDuration(track.Duration)
		s.ProgressSec = parseDuration(track.Progress)

		if track.AlbumArtURI != "" {
			if strings.HasPrefix(track.AlbumArtURI, "http") {
				s.AlbumArtURL = track.AlbumArtURI
			} else {
				s.AlbumArtURL = fmt.Sprintf("http://%s:1400%s", s.IP, track.AlbumArtURI)
			}
		} else {
			s.AlbumArtURL = ""
		}
		return nil
	}
	s.AlbumArtURL = ""
	s.Duration = ""
	s.Progress = ""
	s.DurationSec = 0
	s.ProgressSec = 0

	if s.AudioFormat != "" && s.AudioFormat != "Silence" {
		s.Track = "TV Audio HDMI"
		s.Artist = ""
		return nil
	}

	media, err := s.Player.AVTransport.GetMediaInfo()
	if err == nil {
		uri := strings.ToLower(media.CurrentURI)
		if uri != "" {
			if strings.Contains(uri, "hdmi") || strings.Contains(uri, "htastream") || strings.Contains(uri, "spdif") {
				s.Track = "TV Audio HDMI"
				s.Artist = ""
				return nil
			}
			if strings.Contains(uri, "line-in") || strings.Contains(uri, "linein") {
				s.Track = "Line-In"
				s.Artist = ""
				return nil
			}
			if strings.Contains(uri, "x-rincon:") {
				s.Track = "Grouping"
				s.Artist = ""
				return nil
			}
		}
	}

	if s.IsPlaying {
		if s.AudioFormat != "" {
			s.Track = "TV Audio HDMI"
		} else {
			s.Track = "External Audio"
		}
		s.Artist = ""
	} else {
		s.Track = "Stopped"
		s.Artist = ""
	}

	return nil
}

func (s *Speaker) TogglePlay() error {
	if s.Player == nil { return nil }
	if s.IsPlaying { return s.Player.Pause() }
	return s.Player.Play()
}

func (s *Speaker) SetVolume(vol uint16) error {
	if s.Player == nil { return nil }
	return s.Player.SetVolume(int(vol))
}

func (s *Speaker) ToggleMute() error {
	if s.Player == nil { return nil }
	return s.Player.SetMute(!s.Mute)
}

func (s *Speaker) ToggleNightMode() error {
	if s.Player == nil { return nil }
	val := "1"
	if s.NightMode { val = "0" }
	return s.Player.RenderingControl.SetEQ("NightMode", val)
}

func (s *Speaker) ToggleSpeechEnhancement() error {
	if s.Player == nil { return nil }
	val := "1"
	if s.SpeechEnhancement { val = "0" }
	return s.Player.RenderingControl.SetEQ("DialogLevel", val)
}

func (s *Speaker) ToggleLoudness() error {
	if s.Player == nil { return nil }
	return s.Player.SetLoudness(!s.Loudness)
}

func (s *Speaker) SetBass(val int) error {
	if s.Player == nil { return nil }
	return s.Player.SetBass(val)
}

func (s *Speaker) SetTreble(val int) error {
	if s.Player == nil { return nil }
	return s.Player.SetTreble(val)
}

func (s *Speaker) Next() error {
	if s.Player == nil { return nil }
	return s.Player.Next()
}

func (s *Speaker) Previous() error {
	if s.Player == nil { return nil }
	return s.Player.Previous()
}

func cleanString(s string) string {
	if s == "" {
		return ""
	}
	// If it's a URL or contains query parameters, take only the base name/path
	if strings.Contains(s, "?") {
		s = strings.Split(s, "?")[0]
	}

	// Check if it's a file path or URL-like path
	if strings.Contains(s, "/") {
		parts := strings.Split(s, "/")
		s = parts[len(parts)-1]
	}

	// Remove common audio extensions
	extensions := []string{".mp3", ".aac", ".flac", ".wav", ".m4a", ".ogg"}
	lowerS := strings.ToLower(s)
	for _, ext := range extensions {
		if strings.HasSuffix(lowerS, ext) {
			s = s[:len(s)-len(ext)]
			break
		}
	}

	// Replace underscores with spaces for filenames
	if !strings.Contains(s, " ") && strings.Contains(s, "_") {
		s = strings.ReplaceAll(s, "_", " ")
	}

	// If it's still messy (e.g. contains = or &), it's probably a fallback URL
	if strings.Contains(s, "=") || strings.Contains(s, "&") {
		return "Streaming Audio"
	}

	return strings.TrimSpace(s)
}
