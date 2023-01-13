package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/zanders3/music/pkg/music"
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

func WrapApi[Res any](method string, handle func(req *http.Request) (*Res, error)) func(http.ResponseWriter, *http.Request) {
	writeError := func(message string, code int, w http.ResponseWriter) {
		resBytes, err := json.Marshal(&ErrorRes{Message: message, Code: code})
		if err != nil {
			panic(err)
		}
		w.Header().Add("Content-Type", "application/json")
		_, _ = w.Write(resBytes)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeError("bad method", 400, w)
			return
		}
		res, err := handle(r)
		if err != nil {
			writeError(err.Error(), 500, w)
			return
		}
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
	index music.MusicIndex
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
}

type ListMusicRes struct {
	Results []Result
}

func (m *MusicServer) listMusicFolder(folderPath string, pageToken string) (*ListMusicRes, error) {
	if strings.Contains(folderPath, "..") {
		return nil, NewHttpError(errors.New("bad request"), 400)
	}
	entries, err := os.ReadDir(path.Join(*sourceFolder, path.Clean(folderPath)))
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." {
			continue
		}
		if entry.IsDir() {
			results = append(results, Result{Name: name, Link: "folders/" + path.Join(folderPath, name), Type: ResultType_Folder})
		} else {
			ext := filepath.Ext(name)
			name = name[0 : len(name)-len(ext)]
			if !music.IsMusicFile(ext) {
				continue
			}
			results = append(results, Result{
				Name: name, Type: ResultType_Song,
				Audio: "/content/" + path.Join(folderPath, name) + ext,
			})
		}
	}
	return &ListMusicRes{Results: results}, nil
}

func albumResult(album *music.Album, header bool) Result {
	t := ResultType_Album
	if header {
		t = ResultType_AlbumHeader
	}
	return Result{Name: album.Name, Type: t, Link: "albums/" + album.Name, Artist: album.Artist, Album: album.Name, Image: album.AlbumArtPath}
}

func songResult(album *music.Album, song *music.Song) Result {
	albumArtPath := ""
	if len(album.AlbumArtPath) > 0 {
		albumArtPath = "/content/" + album.AlbumArtPath
	}
	return Result{
		Name: song.Title, Type: ResultType_Song,
		Artist: song.Artist, Album: song.Album, Audio: "/content/" + song.Path, Image: albumArtPath,
	}
}

func (m *MusicServer) ListMusic(r *http.Request) (*ListMusicRes, error) {
	searchType, path, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/music/"), "/")
	pageToken := r.FormValue("pageToken")
	if searchType == "" {
		return &ListMusicRes{Results: []Result{
			{Name: "Artists", Type: ResultType_Folder, Link: "artists"},
			{Name: "Albums", Type: ResultType_Folder, Link: "albums"},
			{Name: "Songs", Type: ResultType_Folder, Link: "songs"},
			{Name: "Folders", Type: ResultType_Folder, Link: "folders"},
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
						for _, song := range m.index.Songs[album.StartSongIdx:album.EndSongIdx] {
							results = append(results, songResult(&album, &song))
						}
					}
					return &ListMusicRes{Results: results}, nil
				}
			}
		}
	} else if searchType == "folders" {
		return m.listMusicFolder(path, pageToken)
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
					for _, song := range m.index.Songs[album.StartSongIdx:album.EndSongIdx] {
						results = append(results, songResult(&album, &song))
					}
					return &ListMusicRes{Results: results}, nil
				}
			}
		}
	} else if searchType == "songs" {
		results := make([]Result, 0, len(m.index.Songs))
		for _, album := range m.index.Albums {
			for _, song := range m.index.Songs[album.StartSongIdx:album.EndSongIdx] {
				results = append(results, songResult(&album, &song))
			}
		}
		return &ListMusicRes{Results: results}, nil
	}
	return nil, NewHttpError(errors.New("bad request"), 400)
}

func main() {
	// clients, errs, err := av1.NewAVTransport1Clients()
	// if err != nil {
	// 	log.Println(err.Error())
	// }
	// for _, err := range errs {
	// 	if err != nil {
	// 		log.Println(err.Error())
	// 	}
	// }
	// for _, client := range clients {
	// 	friendlyName := client.RootDevice.Device.FriendlyName
	// 	for _, device := range client.RootDevice.Device.Devices {
	// 		if device.DeviceType == "urn:schemas-upnp-org:device:MediaRenderer:1" && len(device.FriendlyName) > 0 {
	// 			friendlyName, _, _ = strings.Cut(device.FriendlyName, "-")
	// 			friendlyName = strings.TrimSpace(friendlyName)
	// 		}
	// 	}
	// 	log.Println(friendlyName)
	// 	CurrentTransportState, CurrentTransportStatus, CurrentSpeed, err := client.GetTransportInfo(0)
	// 	if err != nil {
	// 		log.Println(err.Error())
	// 	} else {
	// 		log.Printf("%s: l=%s cts=%s cs=%s", client.Location.String(), CurrentTransportState, CurrentTransportStatus, CurrentSpeed)
	// 	}
	// 	NrTracks, MediaDuration, CurrentURI, CurrentURIMetaData, NextURI, NextURIMetaData, PlayMedium, RecordMedium, WriteStatus, err := client.GetMediaInfo(0)
	// 	if err != nil {
	// 		log.Println(err.Error())
	// 	} else {
	// 		log.Printf("%s: nt=%d md=%s curi=%s curim=%s nuri=%s nurim=%s pm=%s rm=%s ws=%s", client.Location.String(), NrTracks, MediaDuration, CurrentURI, CurrentURIMetaData, NextURI, NextURIMetaData, PlayMedium, RecordMedium, WriteStatus)
	// 	}
	// 	Track, TrackDuration, TrackMetaData, TrackURI, RelTime, AbsTime, RelCount, AbsCount, err := client.GetPositionInfo(0)
	// 	if err != nil {
	// 		log.Println(err.Error())
	// 	} else {
	// 		// tmd=<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="-1" parentID="-1"><res duration="0:05:22"></res><upnp:albumArtURI>http://192.168.1.175:1400/getaa?v=0&amp;vli=1&amp;u=3646428523</upnp:albumArtURI><upnp:class>object.item.audioItem.musicTrack</upnp:class><dc:title>Sleepy Seven</dc:title><dc:creator>Bonobo</dc:creator><upnp:album>Animal Magic</upnp:album><upnp:originalTrackNumber>2</upnp:originalTrackNumber><r:tiid>3519114275655283735</r:tiid></item></DIDL-Lite>
	// 		log.Printf("t=%d td=%s tmd=%s turi=%s rt=%s at=%s rc=%d ac=%d", Track, TrackDuration, TrackMetaData, TrackURI, RelTime, AbsTime, RelCount, AbsCount)
	// 	}
	// 	Actions, err := client.GetCurrentTransportActions(0)
	// 	if err != nil {
	// 		log.Println(err.Error())
	// 	} else {
	// 		log.Println("ac=" + Actions)
	// 	}
	// 	if friendlyName == "Office" {
	// 		client.Pause(0)
	// 		time.Sleep(2 * time.Second)
	// 		client.Play(0, "1")
	// 	}
	// }
	flag.Parse()

	log.Println("music server 🎵 serving music from " + *sourceFolder)
	ms := MusicServer{}
	http.HandleFunc("/api/music/", WrapApi("GET", ms.ListMusic))

	http.Handle("/content/", http.StripPrefix("/content/", http.FileServer(&NoListFs{base: http.Dir(*sourceFolder)})))
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(&NoListFs{base: http.Dir("static/")})))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "index.html") })

	go ms.index.Scan(*sourceFolder)
	log.Println("listening on :3000")
	if err := http.ListenAndServe(":3000", nil); err != nil {
		log.Fatal(err)
	}
}
