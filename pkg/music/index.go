package music

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dhowden/tag"
	"golang.org/x/text/language"
	"golang.org/x/text/search"
)

func IsMusicFile(ext string) bool {
	switch ext {
	case ".mp3", ".m4a", ".aac":
		return true
	default:
		return false
	}
}

type Song struct {
	Path, Title, Artist, Album string
	TrackNum, TrackTotal, Year int
}

type Album struct {
	StartSongIdx, EndSongIdx int
	Name, Artist             string
	AlbumArtPath             string
}

type Artist struct {
	StartAlbumIdx, EndAlbumIdx int
	Name                       string
}

type MusicIndex struct {
	SongsMu sync.Mutex
	Artists []Artist
	Songs   []Song
	Albums  []Album

	AlbumIdByName map[string]int
}

func scanFolder(songs []Song, rootFolder, folder string) ([]Song, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	foundSongs, foundTrackNums := false, false
	numSongs := len(songs)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			var err error
			songs, err = scanFolder(songs, rootFolder, path.Join(folder, name))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
		} else {
			foundSongs = true
			ext := filepath.Ext(name)
			fullPath := path.Join(folder, entry.Name())
			relativePath := strings.ReplaceAll(fullPath, rootFolder, "")
			if !IsMusicFile(ext) {
				switch ext {
				case ".jpg", ".ini", ".DS_Store", ".db", ".png", ".html", ".wpl", ".js", ".pdf", ".m4p", ".wma":
				default:
					log.Printf("found strange file %s", fullPath)
				}
				continue
			}
			f, err := os.Open(fullPath)
			if err != nil {
				f.Close()
				return nil, fmt.Errorf("failed to read %s: %w", fullPath, err)
			}
			m, err := tag.ReadFrom(f)
			if err != nil {
				f.Close()
				return nil, fmt.Errorf("failed to parse %s: %w", fullPath, err)
			}
			trackNum, trackTotal := m.Track()
			artist := m.AlbumArtist()
			if len(artist) == 0 {
				artist = m.Artist()
			}
			songs = append(songs, Song{
				Path:  relativePath,
				Title: m.Title(), Artist: artist, Album: m.Album(),
				TrackNum: trackNum, TrackTotal: trackTotal, Year: m.Year(),
			})
			if trackNum > 0 {
				foundTrackNums = true
			}
			f.Close()
		}
	}
	if foundSongs {
		for i := range songs[numSongs:] {
			song := &songs[i+numSongs]
			if len(song.Artist) == 0 || len(song.Album) == 0 || len(song.Title) == 0 {
				bits := strings.Split(song.Path, "/")
				if len(bits) >= 2 && len(song.Artist) == 0 {
					if len(bits) >= 3 {
						song.Artist = bits[len(bits)-3]
					} else {
						song.Artist = bits[len(bits)-2]
					}
				}
				if len(bits) > 1 && len(song.Album) == 0 {
					song.Album = bits[len(bits)-2]
				}
				if len(song.Title) == 0 {
					name := bits[len(bits)-1]
					ext := filepath.Ext(name)
					name = name[0 : len(name)-len(ext)]
					if len(name) >= 3 && name[0] >= '0' && name[0] <= '9' && name[1] >= '0' && name[1] <= '9' && name[2] == ' ' {
						name = name[3:]
					}
					song.Title = name
				}
			}
			if song.TrackTotal == 0 {
				song.TrackTotal = len(songs) - numSongs
			}
			if !foundTrackNums {
				song.TrackNum = i
			}
		}
	}
	return songs, nil
}

func (mi *MusicIndex) Scan(folder string) {
	log.Println("starting file scan")
	songs, err := scanFolder([]Song{}, folder, folder)
	if err != nil {
		log.Printf("failed to scan: %v", err)
		return
	}
	log.Printf("file scan complete: found %d songs", len(songs))
	// sort the songs and form the album and artist list
	sort.Slice(songs, func(i, j int) bool {
		a, b := &songs[i], &songs[j]
		cmp := strings.Compare(a.Artist, b.Artist)
		if cmp != 0 {
			return cmp < 0
		}
		cmp = strings.Compare(a.Album, b.Album)
		if cmp != 0 {
			return cmp < 0
		}
		if a.TrackNum != b.TrackNum {
			return a.TrackNum < b.TrackNum
		}
		return strings.Compare(a.Path, b.Path) < 0
	})
	var currentArtist, currentAlbum, albumArtPath string
	var albumStartIdx, albumIdx int
	var songStartIdx int
	artists := make([]Artist, 0)
	albums := make([]Album, 0)
	numAlbumArt := 0
	for idx, song := range songs {
		if currentAlbum != song.Album || currentArtist != song.Artist {
			if len(currentAlbum) > 0 {
				albums = append(albums, Album{
					StartSongIdx: songStartIdx, EndSongIdx: idx,
					Name: currentAlbum, Artist: currentArtist,
					AlbumArtPath: albumArtPath,
				})
				albumIdx++
			}
			albumArtPath = path.Join(folder, path.Dir(song.Path), "Folder.jpg")
			if _, err := os.Stat(albumArtPath); err != nil {
				albumArtPath = ""
			} else {
				albumArtPath = albumArtPath[len(folder):]
				numAlbumArt++
			}
			currentAlbum = song.Album
			songStartIdx = idx
		}
		if currentArtist != song.Artist {
			if len(currentArtist) > 0 {
				artists = append(artists, Artist{
					Name: currentArtist, StartAlbumIdx: albumStartIdx, EndAlbumIdx: albumIdx,
				})
			}
			currentArtist = song.Artist
			albumStartIdx = albumIdx
		}
	}
	log.Printf("found %d artists %d albums %d album art", len(artists), len(albums), numAlbumArt)

	albumIdByName := make(map[string]int, len(albums))
	for idx, album := range albums {
		albumIdByName[album.Name] = idx
	}

	mi.SongsMu.Lock()
	mi.Songs, mi.Albums, mi.Artists, mi.AlbumIdByName = songs, albums, artists, albumIdByName
	mi.SongsMu.Unlock()

	alreadyProcessedAlbums := make(map[string]string)
	processedAlbumsFile, err := os.OpenFile(path.Join(folder, "albums.csv"), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Println(err.Error())
		return
	}
	defer processedAlbumsFile.Close()
	{
		albumReader := csv.NewReader(processedAlbumsFile)
		records, err := albumReader.ReadAll()
		if err != nil {
			log.Println(err.Error())
			return
		}
		for _, album := range records {
			alreadyProcessedAlbums[album[0]+":"+album[1]] = album[2]
		}
	}
	processedAlbumsCsv := csv.NewWriter(processedAlbumsFile)
	client := http.Client{}
	for _, album := range albums {
		if len(album.AlbumArtPath) > 0 {
			continue
		}
		albumArtPath := path.Join(folder, path.Dir(songs[album.StartSongIdx].Path), "Folder.jpg")
		mbid, exists := alreadyProcessedAlbums[album.Artist+":"+album.Name]
		if exists {
			continue
		}

		songFile, err := os.Open(path.Join(folder, songs[album.StartSongIdx].Path))
		if err != nil {
			log.Println(err.Error())
			return
		}
		m, err := tag.ReadFrom(songFile)
		if err != nil {
			log.Println(err.Error())
			songFile.Close()
			return
		}
		pic := m.Picture()
		if pic != nil {
			if pic.MIMEType == "image/jpeg" {
				if err := os.WriteFile(albumArtPath, pic.Data, 0644); err != nil {
					songFile.Close()
					log.Println(err.Error())
					return
				}
				album.AlbumArtPath = albumArtPath
				songFile.Close()
				continue
			}
		}
		songFile.Close()

		log.Printf("fetching %s %s", album.Artist, album.Name)
		url := strings.ReplaceAll(fmt.Sprintf("https://musicbrainz.org/ws/2/release?query=artist=%s AND Album=%s&fmt=json&limit=1", album.Artist, album.Name), " ", "%20")
		req, err := http.NewRequest("GET", url, bytes.NewReader([]byte{}))
		req.Header.Add("User-Agent", "MusicThing/0.0.1 ( 3zanders@gmail.com )")
		if err != nil {
			log.Println(err.Error())
			return
		}
		res, err := client.Do(req)
		if err != nil {
			log.Println(err.Error())
			return
		}
		resBytes, err := io.ReadAll(res.Body)
		if err != nil {
			log.Println(err.Error())
			return
		}
		resStr := string(resBytes)
		startIdx := strings.Index(resStr, "\"releases\":[{\"id\":\"")
		if startIdx != -1 {
			endIdx := strings.Index(resStr[startIdx+19:], "\"")
			mbid = resStr[startIdx+19 : startIdx+19+endIdx]
			log.Printf("%s %s has mbid %s", album.Artist, album.Name, mbid)
		} else {
			log.Printf("%s %s has no results", album.Artist, album.Name)
			if err := processedAlbumsCsv.Write([]string{album.Artist, album.Name, mbid}); err != nil {
				log.Println(err.Error())
				return
			}
			processedAlbumsCsv.Flush()
			continue
		}
		imageUrl := fmt.Sprintf("http://coverartarchive.org/release/%s/front", mbid)
		log.Printf("downloading %s %s: %s to %s", album.Artist, album.Name, imageUrl, albumArtPath)
		imageRes, err := http.Get(imageUrl)
		if err != nil {
			log.Println(err.Error())
			return
		}
		if imageRes.StatusCode == 404 {
			log.Println("not found")
			if err := processedAlbumsCsv.Write([]string{album.Artist, album.Name, mbid}); err != nil {
				log.Println(err.Error())
				return
			}
			processedAlbumsCsv.Flush()
			continue
		}
		if imageRes.StatusCode != 200 {
			imageResBytes, _ := io.ReadAll(imageRes.Body)
			log.Printf("%d: %s", res.StatusCode, string(imageResBytes))
			return
		}
		contentType := imageRes.Header.Get("content-type")
		if contentType == "image/jpeg" {
			albumArtFile, err := os.Create(albumArtPath)
			if err != nil {
				log.Println(err.Error())
				return
			}
			if _, err := io.Copy(albumArtFile, imageRes.Body); err != nil {
				log.Println(err.Error())
				albumArtFile.Close()
				return
			}
			defer albumArtFile.Close()
		} else if contentType == "image/png" {
			img, err := png.Decode(imageRes.Body)
			if err != nil {
				log.Println(err.Error())
				return
			}
			newImg := image.NewRGBA(img.Bounds())
			draw.Draw(newImg, newImg.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
			draw.Draw(newImg, newImg.Bounds(), img, image.Point{}, draw.Src)
			albumArtFile, err := os.Create(albumArtPath)
			if err != nil {
				log.Println(err.Error())
				return
			}
			if err := jpeg.Encode(albumArtFile, newImg, &jpeg.Options{Quality: 80}); err != nil {
				log.Println(err.Error())
				albumArtFile.Close()
				return
			}
			albumArtFile.Close()
		} else {
			log.Printf("bad image: %s", contentType)
			return
		}
		log.Printf("downloaded ok!")
		if err := processedAlbumsCsv.Write([]string{album.Artist, album.Name, mbid}); err != nil {
			log.Println(err.Error())
			return
		}
		processedAlbumsCsv.Flush()
		time.Sleep(time.Second)
		album.AlbumArtPath = albumArtPath
	}
	log.Println("album art processing complete")
}

type SearchResult struct {
	SongId, AlbumId, ArtistId int
}

const MaxSearchResults = 300

func (i *MusicIndex) Search(s string) []SearchResult {
	pattern := search.New(language.English, search.IgnoreCase).CompileString(s)

	results := make([]SearchResult, 0)
	for idx, artist := range i.Artists {
		if i, _ := pattern.IndexString(artist.Name); i != -1 {
			results = append(results, SearchResult{ArtistId: idx, SongId: -1, AlbumId: -1})
			if len(results) > MaxSearchResults {
				break
			}
		}
	}
	for idx, album := range i.Albums {
		if i, _ := pattern.IndexString(album.Name); i != -1 {
			results = append(results, SearchResult{AlbumId: idx, SongId: -1, ArtistId: -1})
			if len(results) > MaxSearchResults {
				break
			}
		}
	}
	for idx, song := range i.Songs {
		if i, _ := pattern.IndexString(song.Title); i != -1 {
			results = append(results, SearchResult{SongId: idx, AlbumId: -1, ArtistId: -1})
			if len(results) > MaxSearchResults {
				break
			}
		}
	}
	return results
}
