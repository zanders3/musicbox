package music

import (
	"log"
	"sync"
	"time"

	"github.com/szatmary/sonos"
)

type Sonos struct {
	ZonePlayers   []*sonos.ZonePlayer
	ZonePlayersMu sync.RWMutex
}

func NewSonos() *Sonos {
	s := &Sonos{}
	go s.search()
	return s
}

func (s *Sonos) search() {
	sn, err := sonos.NewSonos()
	if err != nil {
		log.Println(err)
		return
	}
	for {
		zns, err := sn.Search()
		if err != nil {
			log.Printf("failed to search for sonos trying again in 10 seconds: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}
		for zp := range zns {
			log.Println("found sonos: " + zp.RoomName())
			s.ZonePlayersMu.Lock()
			s.ZonePlayers = append(s.ZonePlayers, zp)
			s.ZonePlayersMu.Unlock()
		}
		return
	}
}
