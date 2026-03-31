package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

type EventServer struct {
	Port     int
	LocalIP  string
	Speakers map[string]*Speaker
	mu       sync.RWMutex
}

func NewEventServer(speakers []*Speaker) (*EventServer, error) {
	ip, err := getLocalIP()
	if err != nil {
		return nil, err
	}

	es := &EventServer{
		LocalIP:  ip,
		Speakers: make(map[string]*Speaker),
	}

	for _, s := range speakers {
		es.Speakers[s.IP] = s
	}

	return es, nil
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("could not find local IP")
}

func (es *EventServer) Start() error {
	listener, err := net.Listen("tcp", ":0") // Random port
	if err != nil {
		return err
	}
	es.Port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/notify", es.handleNotify)

	go http.Serve(listener, mux)

	// Subscribe all speakers
	for _, s := range es.Speakers {
		go es.Subscribe(s)
	}

	return nil
}

func (es *EventServer) Subscribe(s *Speaker) {
	services := []string{
		"/MediaRenderer/AVTransport/Event",
		"/MediaRenderer/RenderingControl/Event",
		"/ZoneGroupTopology/Event",
	}

	callback := fmt.Sprintf("<http://%s:%d/notify>", es.LocalIP, es.Port)

	for _, svc := range services {
		url := fmt.Sprintf("http://%s:1400%s", s.IP, svc)
		req, _ := http.NewRequest("SUBSCRIBE", url, nil)
		req.Header.Set("CALLBACK", callback)
		req.Header.Set("NT", "upnp:event")
		req.Header.Set("TIMEOUT", "Second-1800")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}
}

func (es *EventServer) handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != "NOTIFY" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	// Find speaker by IP
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	es.mu.RLock()
	s, ok := es.Speakers[ip]
	es.mu.RUnlock()

	if ok {
		// Sonos events are wrapped in <e:propertyset>
		// The internal state is usually XML-encoded inside <LastChange>
		xmlStr := string(body)
		
		// Any valid property update is worth a refresh
		if strings.Contains(xmlStr, "<e:propertyset") {
			go func() {
				s.UpdateStatus()
				if s.OnUpdate != nil {
					s.OnUpdate()
				}
			}()
		}
	}

	w.WriteHeader(http.StatusOK)
}
