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
	"sort"
	"strings"

	avtransport "github.com/szatmary/sonos/AVTransport"
	"github.com/zanders3/music/pkg/music"
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
	index        music.MusicIndex
	sonos        *music.Sonos
	internalAddr string
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
				results = append(results, Result{
					Name: artist.Name, Type: ResultType_Artist,
					Artist: artist.Name,
					Link:   "artists/" + artist.Name,
				})
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
	Track, Artist, Album string
	Position, Duration   string
	AlbumArtURI          string
	Playing              bool
	Volume               int
}

type ListSonosRes struct {
	Rooms []string    `json:",omitempty"`
	Sonos *SonosState `json:",omitempty"`
}

type AlbumMeta struct {
	Item []AlbumMetaItem `xml:"item"`
}

type AlbumMetaItem struct {
	Title        string   `xml:"http://purl.org/dc/elements/1.1/ title,omitempty"`
	Artist       string   `xml:"http://purl.org/dc/elements/1.1/ creator,omitempty"`
	Albums       []string `xml:"urn:schemas-upnp-org:metadata-1-0/upnp/ album,omitempty"`
	AlbumArtURIs []string `xml:"urn:schemas-upnp-org:metadata-1-0/upnp/ albumArtURI,omitempty"`
}

type PlayRequest struct {
	SongIDs []int
	Volume  *int
}

func (m *MusicServer) toSonosSongUri(songId int) string {
	return m.internalAddr + "/content" + strings.ReplaceAll(m.index.Songs[songId].Path, " ", "%20")
}

func (m *MusicServer) ListSonos(req *http.Request) (*ListSonosRes, error) {
	sonosName := strings.TrimPrefix(req.URL.Path, "/api/sonos/")
	m.sonos.ZonePlayersMu.Lock()
	defer m.sonos.ZonePlayersMu.Unlock()
	if len(sonosName) > 0 {
		for _, zp := range m.sonos.ZonePlayers {
			if zp.RoomName() == sonosName {
				if req.Method == "POST" {
					var playReq PlayRequest
					if err := json.NewDecoder(req.Body).Decode(&playReq); err != nil {
						return nil, NewHttpError(err, 400)
					}
					if playReq.Volume != nil && *playReq.Volume >= 0 && *playReq.Volume <= 100 {
						if err := zp.SetVolume(*playReq.Volume); err != nil {
							return nil, err
						}
					}
					if len(playReq.SongIDs) > 0 {
						// if _, err := zp.AVTransport.SetAVTransportURI(zp.HttpClient, &avtransport.SetAVTransportURIArgs{
						// 	InstanceID: 0, CurrentURI: m.toSonosSongUri(playReq.SongIDs[0]),
						// }); err != nil {
						// 	return nil, err
						// }
						// if _, err := zp.AVTransport.Play(zp.HttpClient, &avtransport.PlayArgs{InstanceID: 0, Speed: "1"}); err != nil {
						// 	return nil, err
						// }
						if _, err := zp.AVTransport.RemoveAllTracksFromQueue(zp.HttpClient, &avtransport.RemoveAllTracksFromQueueArgs{InstanceID: 0}); err != nil {
							return nil, err
						}
						for _, songId := range playReq.SongIDs {
							if _, err := zp.AVTransport.AddURIToQueue(zp.HttpClient, &avtransport.AddURIToQueueArgs{InstanceID: 0, EnqueuedURI: m.toSonosSongUri(songId)}); err != nil {
								return nil, err
							}
							// batchSize := 16
							// if len(playReq.SongIDs) < batchSize {
							// 	batchSize = len(playReq.SongIDs)
							// }
							// var uris strings.Builder
							// for idx, songId := range playReq.SongIDs[0:batchSize] {
							// 	if idx > 0 {
							// 		uris.WriteString(" ")
							// 	}
							// 	uris.WriteString(m.toSonosSongUri(songId))
							// }
							// uriStr := uris.String()
							// playReq.SongIDs = playReq.SongIDs[batchSize:]
							// if _, err := zp.AVTransport.AddMultipleURIsToQueue(zp.HttpClient, &avtransport.AddMultipleURIsToQueueArgs{
							// 	InstanceID: 0, UpdateID: 0, NumberOfURIs: uint32(batchSize), EnqueuedURIs: uriStr, ContainerURI: "", ContainerMetaData: "",
							// 	DesiredFirstTrackNumberEnqueued: 0, EnqueuedURIsMetaData: "", EnqueueAsNext: false,
							// }); err != nil {
							// 	return nil, err
							// }
						}
						if _, err := zp.AVTransport.Play(zp.HttpClient, &avtransport.PlayArgs{InstanceID: 0, Speed: "1"}); err != nil {
							return nil, err
						}
					}
					return &ListSonosRes{}, nil
				} else if req.Method == "GET" {
					res, err := zp.AVTransport.GetPositionInfo(http.DefaultClient, &avtransport.GetPositionInfoArgs{InstanceID: 0})
					if err != nil {
						return nil, err
					}
					var artist, album, title, albumArtUri string
					if len(res.TrackMetaData) > 0 {
						var albumMeta AlbumMeta
						if err := xml.Unmarshal([]byte(res.TrackMetaData), &albumMeta); err != nil {
							return nil, err
						}
						if len(albumMeta.Item) > 0 {
							artist, title = albumMeta.Item[0].Artist, albumMeta.Item[0].Title
							if len(albumMeta.Item[0].Albums) > 0 {
								album = albumMeta.Item[0].Albums[0]
							}
							if len(albumMeta.Item[0].AlbumArtURIs) > 0 {
								albumArtUri = albumMeta.Item[0].AlbumArtURIs[0]
							}
						}
					}
					vol, err := zp.GetVolume()
					if err != nil {
						return nil, err
					}
					tres, err := zp.AVTransport.GetTransportInfo(http.DefaultClient, &avtransport.GetTransportInfoArgs{InstanceID: 0})
					if err != nil {
						return nil, err
					}
					return &ListSonosRes{
						Sonos: &SonosState{Artist: artist, Album: album, Track: title, Position: res.RelTime, AlbumArtURI: albumArtUri, Duration: res.TrackDuration, Volume: vol, Playing: tres.CurrentTransportState == "PLAYING"},
					}, nil
				}
			}
		}
		return nil, NewHttpError(fmt.Errorf("not found"), 404)
	} else if req.Method == "GET" {
		rooms := make([]string, 0, len(m.sonos.ZonePlayers))
		for _, zp := range m.sonos.ZonePlayers {
			rooms = append(rooms, zp.RoomName())
		}
		sort.Strings(rooms)
		return &ListSonosRes{Rooms: rooms}, nil
	}
	return nil, NewHttpError(fmt.Errorf("bad method"), 400)
}

func intercept(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	})
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
			internalAddr = "http://" + addrStr + ":3000"
			break
		}
	}

	log.Println("music server 🎵 serving music from " + *sourceFolder + " at " + internalAddr)
	ms := MusicServer{}
	ms.sonos = music.NewSonos()
	ms.internalAddr = internalAddr

	http.HandleFunc("/api/music/", WrapApi(ms.ListMusic))
	http.HandleFunc("/api/sonos/", WrapApi(ms.ListSonos))

	http.Handle("/content/", intercept(http.StripPrefix("/content/", http.FileServer(&NoListFs{base: http.Dir(*sourceFolder)}))))
	static.ServeHTML()

	go ms.index.Scan(*sourceFolder)
	log.Println("listening on :3000")
	if err := http.ListenAndServe(":3000", nil); err != nil {
		log.Fatal(err)
	}
}
