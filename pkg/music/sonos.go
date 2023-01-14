package music

import (
	"log"
	"sync"

	"github.com/szatmary/sonos"
)

type Sonos struct {
	ZonePlayers   []*sonos.ZonePlayer
	ZonePlayersMu sync.Mutex
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
	zns, err := sn.Search()
	if err != nil {
		log.Fatal(err)
	}
	for zp := range zns {
		log.Println("found sonos: " + zp.RoomName())
		s.ZonePlayersMu.Lock()
		s.ZonePlayers = append(s.ZonePlayers, zp)
		s.ZonePlayersMu.Unlock()
	}
}
