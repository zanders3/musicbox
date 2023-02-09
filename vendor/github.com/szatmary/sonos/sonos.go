package sonos

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	mx        = 5
	st        = "urn:schemas-upnp-org:device:ZonePlayer:1"
	bcastaddr = "239.255.255.250:1900"
)

type Sonos struct {
	// Context Context
	listenSocket *net.UDPConn
	udpReader    *bufio.Reader
	found        chan *ZonePlayer
}

func NewSonos() (*Sonos, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: []byte{0, 0, 0, 0}, Port: 0, Zone: ""})
	if err != nil {
		return nil, err
	}

	s := Sonos{
		listenSocket: conn,
		udpReader:    bufio.NewReader(conn),
		found:        make(chan *ZonePlayer),
	}
	go func() {
		for {
			response, err := http.ReadResponse(s.udpReader, nil)
			if err != nil {
				time.Sleep(10 * time.Second)
				continue
			}

			location, err := url.Parse(response.Header.Get("Location"))
			if err != nil {
				time.Sleep(10 * time.Second)
				continue
			}
			zp, err := NewZonePlayer(location)
			if err != nil {
				time.Sleep(10 * time.Second)
				continue
			}
			if zp.IsCoordinator() {
				s.found <- zp
			}
		}
	}()
	return &s, nil
}

func (s *Sonos) Search() (chan *ZonePlayer, error) {
	// MX should be set to use timeout value in integer seconds
	pkt := []byte(fmt.Sprintf("M-SEARCH * HTTP/1.1\r\nHOST: %s\r\nMAN: \"ssdp:discover\"\r\nMX: %d\r\nST: %s\r\n\r\n", bcastaddr, mx, st))
	bcast, err := net.ResolveUDPAddr("udp", bcastaddr)
	if err != nil {
		return nil, err
	}
	_, err = s.listenSocket.WriteTo(pkt, bcast)
	if err != nil {
		return nil, err
	}
	return s.found, nil
}
