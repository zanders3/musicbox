package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/szatmary/sonos"
	avtransport "github.com/szatmary/sonos/AVTransport"
	"github.com/zanders3/music/pkg/music"
	"github.com/zanders3/music/pkg/sonosevs"
	"github.com/zanders3/music/static"
)

var sourceFolder = flag.String("folder", "D:\\Music", "where the music is hosted")

type HttpError struct {
	err  error
	code int
}

func (h *HttpError) Error() string {
	return h.err.Error()
}

func (h *HttpError) Unwrap() error {
	return h.err
}

func NewHttpError(err error, code int) error {
	return &HttpError{err: err, code: code}
}

type ErrorRes struct {
	Code    int
	Message string
}

func WrapApi[Res any](handle func(req *http.Request) (*Res, error)) func(http.ResponseWriter, *http.Request) {
	writeError := func(message string, code int, w http.ResponseWriter) {
		resBytes, err := json.Marshal(&ErrorRes{Message: message, Code: code})
		if err != nil {
			panic(err)
		}
		w.Header().Add("Content-Type", "application/json")
		_, _ = w.Write(resBytes)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := handle(r)
		if err != nil {
			var httpError *HttpError
			if errors.As(err, &httpError) {
				writeError(err.Error(), httpError.code, w)
				return
			}
			writeError(err.Error(), 500, w)
			return
		}
		resBytes, err := json.Marshal(res)
		if err != nil {
			writeError(err.Error(), 500, w)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		_, _ = w.Write(resBytes)
	}
}

type NoListFs struct {
	base http.FileSystem
}

func (n *NoListFs) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}

func (n *NoListFs) Open(name string) (http.File, error) {
	if !strings.Contains(name, ".") {
		return n.base.Open("does-not-exist")
	}
	return n.base.Open(name)
}

type MusicServer struct {
	index         music.MusicIndex
	sonos         *music.Sonos
	subscriptions *music.Subscriptions
	internalAddr  string
}

type ResultType string

const (
	ResultType_Song        ResultType = "Song"
	ResultType_Artist      ResultType = "Artist"
	ResultType_Album       ResultType = "Album"
	ResultType_AlbumHeader ResultType = "AlbumHeader"
	ResultType_Folder      ResultType = "Folder"
)

type Result struct {
	Name                 string
	Type                 ResultType
	Link, Audio          string
	Artist, Album, Image string
	SongId               int
}

type ListMusicRes struct {
	Results []Result
}

func albumResult(album *music.Album, header bool) Result {
	t := ResultType_Album
	if header {
		t = ResultType_AlbumHeader
	}
	return Result{Name: album.Name, Type: t, Link: "albums/" + album.Name, Artist: album.Artist, Album: album.Name, Image: album.AlbumArtPath}
}

func (m *MusicServer) songResult(album *music.Album, songIdx int) Result {
	albumArtPath := ""
	if len(album.AlbumArtPath) > 0 {
		albumArtPath = "/content" + album.AlbumArtPath
	}
	song := m.index.Songs[songIdx]
	return Result{
		Name: song.Title, Type: ResultType_Song, SongId: songIdx,
		Artist: song.Artist, Album: song.Album, Audio: "/content" + song.Path, Image: albumArtPath,
	}
}

func artistResult(artist *music.Artist) Result {
	return Result{
		Name: artist.Name, Type: ResultType_Artist,
		Artist: artist.Name,
		Link:   "artists/" + artist.Name,
	}
}

func (m *MusicServer) ListMusic(r *http.Request) (*ListMusicRes, error) {
	if r.Method != "GET" {
		return nil, NewHttpError(fmt.Errorf("bad method"), 400)
	}
	searchType, path, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/music/"), "/")
	if searchType == "" {
		return &ListMusicRes{Results: []Result{
			{Name: "Artists", Type: ResultType_Folder, Link: "artists"},
			{Name: "Albums", Type: ResultType_Folder, Link: "albums"},
			{Name: "Songs", Type: ResultType_Folder, Link: "songs"},
		}}, nil
	} else if searchType == "artists" {
		m.index.SongsMu.Lock()
		defer m.index.SongsMu.Unlock()
		if len(path) == 0 {
			results := make([]Result, 0, len(m.index.Artists))
			for _, artist := range m.index.Artists {
				results = append(results, artistResult(&artist))
			}
			return &ListMusicRes{Results: results}, nil
		} else {
			results := make([]Result, 0)
			for _, artist := range m.index.Artists {
				if artist.Name == path {
					for _, album := range m.index.Albums[artist.StartAlbumIdx:artist.EndAlbumIdx] {
						results = append(results, albumResult(&album, true))
						for songIdx := album.StartSongIdx; songIdx < album.EndSongIdx; songIdx++ {
							results = append(results, m.songResult(&album, songIdx))
						}
					}
					return &ListMusicRes{Results: results}, nil
				}
			}
		}
	} else if searchType == "albums" {
		if len(path) == 0 {
			results := make([]Result, 0, len(m.index.Albums))
			for _, album := range m.index.Albums {
				results = append(results, albumResult(&album, false))
			}
			return &ListMusicRes{Results: results}, nil
		} else {
			results := make([]Result, 0)
			for _, album := range m.index.Albums {
				if album.Name == path {
					results = append(results, albumResult(&album, true))
					for songIdx := album.StartSongIdx; songIdx < album.EndSongIdx; songIdx++ {
						results = append(results, m.songResult(&album, songIdx))
					}
					return &ListMusicRes{Results: results}, nil
				}
			}
		}
	} else if searchType == "songs" {
		results := make([]Result, 0, len(m.index.Songs))
		for _, album := range m.index.Albums {
			for songIdx := album.StartSongIdx; songIdx < album.EndSongIdx; songIdx++ {
				results = append(results, m.songResult(&album, songIdx))
			}
		}
		return &ListMusicRes{Results: results}, nil
	}
	return nil, NewHttpError(errors.New("bad request"), 400)
}

type SonosState struct {
	Track, Artist, Album string `json:",omitempty"`
	Position, Duration   string `json:",omitempty"`
	AlbumArtURI          string `json:",omitempty"`
	Playing              *bool  `json:",omitempty"`
	Volume               *int   `json:",omitempty"`
}

type ListSonosRes struct {
	Rooms []string    `json:",omitempty"`
	Sonos *SonosState `json:",omitempty"`
}

type ActionRequest struct {
	SongIDs     []int
	Volume      *int
	SetTimeSecs *int
	Action      string // Play, Pause, Next, Prev
}

func (m *MusicServer) toSonosSongUri(songId int) string {
	return m.internalAddr + "/content" + strings.ReplaceAll(m.index.Songs[songId].Path, " ", "%20")
}

func (m *MusicServer) toSonosSongMetadata(songId int) string {
	songUri := m.toSonosSongUri(songId)
	song := m.index.Songs[songId]
	var albumArtUri string
	if album, ok := m.index.AlbumIdByName[song.Album]; ok && len(m.index.Albums[album].AlbumArtPath) > 0 {
		albumArtUri = m.internalAddr + "/content" + strings.ReplaceAll(m.index.Albums[album].AlbumArtPath, " ", "%20")
	}
	songExt := path.Ext(songUri)
	durationStr := fmt.Sprintf("%02d:%02d:%02d", song.DurationSecs/(60*60), song.DurationSecs/60, song.DurationSecs%60)
	s := fmt.Sprintf("<DIDL-Lite xmlns:dc=\"http://purl.org/dc/elements/1.1/\" xmlns:upnp=\"urn:schemas-upnp-org:metadata-1-0/upnp/\" xmlns:r=\"urn:schemas-rinconnetworks-com:metadata-1-0/\" xmlns=\"urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/\"><item id=\"-1\" parentID=\"-1\" restricted=\"true\"><res protocolInfo=\"http-get:*:audio/%s:*\" duration=\"%s\">%s</res><r:streamContent></r:streamContent><r:radioShowMd></r:radioShowMd><r:streamInfo>bd:16,sr:44100,c:3,l:0,d:0</r:streamInfo><dc:title>%s</dc:title><upnp:class>object.item.audioItem.musicTrack</upnp:class><dc:creator>%s</dc:creator><upnp:album>%s</upnp:album><upnp:originalTrackNumber>4</upnp:originalTrackNumber><r:narrator>%s</r:narrator><r:albumArtist>%s</r:albumArtist><upnp:albumArtURI>%s</upnp:albumArtURI></item></DIDL-Lite>",
		songExt,
		durationStr,
		songUri,
		song.Title,
		song.Artist,
		song.Album,
		song.Artist,
		song.Artist,
		albumArtUri,
	)
	log.Println(s)
	return s
}

func (m *MusicServer) GetSonos(w http.ResponseWriter, zp *sonos.ZonePlayer, req *http.Request) {
	ctx := req.Context()
	unsubscribe := make(chan struct{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	m.subscriptions.Subscribe(zp.AVTransport.EventEndpoint, unsubscribe, func(e string) {
		var ev sonosevs.AudioTransportEvent
		xml.Unmarshal([]byte(e), &ev)
		var didl sonosevs.DIDLLite
		xml.Unmarshal([]byte(ev.InstanceID.CurrentTrackMetaData.Val), &didl)
		pos, err := zp.AVTransport.GetPositionInfo(http.DefaultClient, &avtransport.GetPositionInfoArgs{InstanceID: 0})
		if err != nil {
			close(unsubscribe)
			return
		}
		playing := ev.InstanceID.TransportState.Val == "PLAYING"
		var albumArtURI string
		if len(didl.Item.AlbumArtURI) > 0 {
			if strings.HasPrefix(didl.Item.AlbumArtURI, "http://") {
				albumArtURI = didl.Item.AlbumArtURI
			} else {
				albumArtURI = "http://" + zp.AVTransport.ControlEndpoint.Host + didl.Item.AlbumArtURI
			}
		}
		resBytes, err := json.Marshal(&ListSonosRes{
			Sonos: &SonosState{
				Track:       didl.Item.Title,
				Artist:      didl.Item.Creator,
				Album:       didl.Item.Album,
				Duration:    ev.InstanceID.CurrentTrackDuration.Val,
				Playing:     &playing,
				Position:    pos.RelTime,
				AlbumArtURI: albumArtURI,
			},
		})
		if err != nil {
			close(unsubscribe)
			return
		}
		finalBytes := append([]byte("data: "), resBytes...)
		finalBytes = append(finalBytes, []byte("\n\n")...)
		if _, err := w.Write(finalBytes); err != nil {
			close(unsubscribe)
			return
		}
		w.(http.Flusher).Flush()
	})
	m.subscriptions.Subscribe(zp.RenderingControl.EventEndpoint, unsubscribe, func(e string) {
		var ev sonosevs.RenderingControlEvent
		xml.Unmarshal([]byte(e), &ev)
		var volume int64
		if len(ev.InstanceID.Volume) > 0 {
			volume, _ = strconv.ParseInt(ev.InstanceID.Volume[0].Val, 10, 32)
		}
		vol := int(volume)
		resBytes, err := json.Marshal(&ListSonosRes{
			Sonos: &SonosState{Volume: &vol},
		})
		if err != nil {
			close(unsubscribe)
			return
		}
		finalBytes := append([]byte("data: "), resBytes...)
		finalBytes = append(finalBytes, []byte("\n\n")...)
		if _, err := w.Write(finalBytes); err != nil {
			close(unsubscribe)
			return
		}
		w.(http.Flusher).Flush()
	})
	select {
	case <-ctx.Done():
		close(unsubscribe)
	case <-unsubscribe:
	}
}

func (m *MusicServer) ListSonos(w http.ResponseWriter, req *http.Request) {
	sonosName, bit, _ := strings.Cut(strings.TrimPrefix(req.URL.Path, "/api/sonos/"), "/")
	m.sonos.ZonePlayersMu.RLock()
	defer m.sonos.ZonePlayersMu.RUnlock()
	if len(sonosName) > 0 {
		for _, zp := range m.sonos.ZonePlayers {
			if zp.RoomName() == sonosName {
				if req.Method == "POST" && bit == "action" {
					WrapApi(func(req *http.Request) (*ListSonosRes, error) {
						var actionReq ActionRequest
						if err := json.NewDecoder(req.Body).Decode(&actionReq); err != nil {
							return nil, NewHttpError(err, 400)
						}
						if actionReq.Volume != nil && *actionReq.Volume >= 0 && *actionReq.Volume <= 100 {
							if err := zp.SetVolume(*actionReq.Volume); err != nil {
								return nil, err
							}
						}
						switch actionReq.Action {
						case "Play":
							if _, err := zp.AVTransport.Play(zp.HttpClient, &avtransport.PlayArgs{InstanceID: 0, Speed: "1"}); err != nil {
								return nil, err
							}
						case "Pause":
							if _, err := zp.AVTransport.Pause(zp.HttpClient, &avtransport.PauseArgs{InstanceID: 0}); err != nil {
								return nil, err
							}
						case "Next":
							if _, err := zp.AVTransport.Next(zp.HttpClient, &avtransport.NextArgs{InstanceID: 0}); err != nil {
								return nil, err
							}
						case "Prev":
							if _, err := zp.AVTransport.Previous(zp.HttpClient, &avtransport.PreviousArgs{InstanceID: 0}); err != nil {
								return nil, err
							}
						}
						if len(actionReq.SongIDs) > 0 {
							if _, err := zp.AVTransport.RemoveAllTracksFromQueue(zp.HttpClient, &avtransport.RemoveAllTracksFromQueueArgs{InstanceID: 0}); err != nil {
								return nil, err
							}
							if len(actionReq.SongIDs) > 30 {
								actionReq.SongIDs = actionReq.SongIDs[0:30]
							}
							for _, songId := range actionReq.SongIDs {
								if _, err := zp.AVTransport.AddURIToQueue(zp.HttpClient, &avtransport.AddURIToQueueArgs{InstanceID: 0, EnqueuedURI: m.toSonosSongUri(songId), EnqueuedURIMetaData: m.toSonosSongMetadata(songId)}); err != nil {
									return nil, err
								}
							}
							udn := strings.TrimPrefix(zp.Root.Device.UDN, "uuid:")
							if _, err := zp.AVTransport.SetAVTransportURI(http.DefaultClient, &avtransport.SetAVTransportURIArgs{InstanceID: 0, CurrentURI: "x-rincon-queue:" + udn + "#0"}); err != nil {
								return nil, err
							}
							if _, err := zp.AVTransport.Play(zp.HttpClient, &avtransport.PlayArgs{InstanceID: 0, Speed: "1"}); err != nil {
								return nil, err
							}
						}
						if actionReq.SetTimeSecs != nil {
							seekStr := fmt.Sprintf("%02d:%02d:%02d", *actionReq.SetTimeSecs/(60*60), *actionReq.SetTimeSecs/60, *actionReq.SetTimeSecs%60)
							if _, err := zp.AVTransport.Seek(zp.HttpClient, &avtransport.SeekArgs{InstanceID: 0, Unit: "REL_TIME", Target: seekStr}); err != nil {
								return nil, err
							}
						}
						return &ListSonosRes{}, nil
					})(w, req)
					return
				} else if req.Method == "GET" && bit == "events" {
					m.GetSonos(w, zp, req)
					return
				}
			}
		}
		WrapApi(func(req *http.Request) (*ListSonosRes, error) {
			return nil, NewHttpError(fmt.Errorf("not found"), 404)
		})(w, req)
	} else {
		WrapApi(func(req *http.Request) (*ListSonosRes, error) {
			if req.Method == "GET" {
				rooms := make([]string, 0, len(m.sonos.ZonePlayers))
				for _, zp := range m.sonos.ZonePlayers {
					rooms = append(rooms, zp.RoomName())
				}
				sort.Strings(rooms)
				return &ListSonosRes{Rooms: rooms}, nil
			}
			return nil, NewHttpError(fmt.Errorf("bad method"), 400)
		})(w, req)
	}
}

type SearchResponse struct {
	Results []Result
}

func (m *MusicServer) SearchMusic(req *http.Request) (*SearchResponse, error) {
	res := m.index.Search(strings.TrimPrefix(req.URL.Path, "/api/search/"))
	results := make([]Result, len(res))
	for idx, r := range res {
		if r.SongId != -1 {
			song := m.index.Songs[r.SongId]
			album := &m.index.Albums[m.index.AlbumIdByName[song.Album]]
			results[idx] = m.songResult(album, r.SongId)
		} else if r.AlbumId != -1 {
			results[idx] = albumResult(&m.index.Albums[r.AlbumId], false)
		} else if r.ArtistId != -1 {
			results[idx] = artistResult(&m.index.Artists[r.ArtistId])
		}
	}
	return &SearchResponse{Results: results}, nil
}

func main() {
	flag.Parse()

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal(err)
	}
	var internalAddr string
	for _, addr := range addrs {
		addrStr, _, _ := strings.Cut(addr.String(), "/")
		if strings.HasPrefix(addrStr, "192.") {
			internalAddr = addrStr
			break
		}
	}

	log.Println("music server 🎵 serving music from " + *sourceFolder + " at http://" + internalAddr + ":3000")
	ms := MusicServer{}
	ms.sonos = music.NewSonos()
	ms.internalAddr = "http://" + internalAddr + ":3000"
	ms.subscriptions = music.ListenForSubscriptionEvents(internalAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/music/", WrapApi(ms.ListMusic))
	mux.HandleFunc("/api/sonos/", ms.ListSonos)
	mux.HandleFunc("/api/search/", WrapApi(ms.SearchMusic))
	mux.Handle("/content/", http.StripPrefix("/content/", http.FileServer(&NoListFs{base: http.Dir(*sourceFolder)})))
	static.ServeHTML(mux)

	go ms.index.Scan(*sourceFolder)
	log.Println("listening on :3000")
	if err := http.ListenAndServe(":3000", mux); err != nil {
		log.Fatal(err)
	}
}
