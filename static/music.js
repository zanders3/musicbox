(() => {
  // music.ts
  function el(id) {
    return document.getElementById(id);
  }
  var audio = document.createElement("audio");
  var nextaudio = document.createElement("audio");
  var is_playing = false;
  var enable_range_update = true;
  var enable_volume_update = true;
  var unmute_volume = audio.volume;
  audio.onplay = function() {
    el("player-play").innerHTML = `<i class="material-icons">pause</i>`;
    is_playing = true;
  };
  audio.onpause = function() {
    el("player-play").innerHTML = `<i class="material-icons">play_arrow</i>`;
    is_playing = false;
  };
  function formatTime(time) {
    let mins = Math.floor(time / 60).toString();
    let secs = Math.floor(time % 60).toString();
    if (secs.length < 2) {
      secs = "0" + secs;
    }
    return mins + ":" + secs;
  }
  function parseTime(time) {
    let bits = time.split(":");
    if (bits.length != 3) {
      return 1;
    }
    return parseInt(bits[0]) * 60 * 60 + parseInt(bits[1]) * 60 + parseInt(bits[2]);
  }
  audio.ontimeupdate = function() {
    if (enable_range_update) {
      el("player-curtime").innerText = formatTime(audio.currentTime);
      el("player-range").value = audio.currentTime.toString();
    } else {
      el("player-curtime").innerText = formatTime(parseFloat(el("player-range").value));
    }
    if (audio.currentTime >= audio.duration) {
      nexttrack();
    }
  };
  audio.onloadedmetadata = function() {
    el("player-endtime").innerText = formatTime(audio.duration);
    let range = el("player-range");
    range.max = audio.duration.toString();
    range.value = "0";
    el("player-curtime").innerText = "0:00";
  };
  audio.onvolumechange = function() {
    console.log(audio.volume);
    el("player-volume").value = (audio.volume * 100).toString();
    el("player-mute").innerHTML = `<i class="material-icons">${audio.volume > 0 ? "volume_up" : "volume_mute"}</i>`;
  };
  var sonosRoom = "";
  var playlist = [];
  var playlistIdx = 0;
  function nexttrack() {
    if (playlistIdx < playlist.length && sonosRoom.length == 0) {
      playlistIdx++;
      playsong();
    }
    if (sonosRoom.length > 0) {
      sonoscommand({ Action: "Next" });
    }
  }
  function prevtrack() {
    if (playlistIdx > 0 && sonosRoom.length == 0) {
      playlistIdx--;
      playsong();
    }
    if (sonosRoom.length > 0) {
      sonoscommand({ Action: "Prev" });
    }
  }
  function sonoscommand(actionReq) {
    var req = new XMLHttpRequest();
    req.open("POST", "/api/sonos/" + sonosRoom + "/action");
    req.onload = function() {
      console.log(req.response);
    };
    let songIds = [];
    for (let idx = playlistIdx; idx < playlist.length; idx++) {
      songIds.push(playlist[idx].SongId);
    }
    req.send(JSON.stringify(actionReq));
  }
  function playsong() {
    if (playlistIdx < 0 || playlist.length == 0 || playlistIdx >= playlist.length) {
      audio.pause();
      return;
    }
    let song = playlist[playlistIdx];
    console.log("play " + song.Audio + " on " + sonosRoom);
    if (sonosRoom.length == 0) {
      audio.src = song.Audio;
      audio.load();
      audio.play();
      el("player-info").innerHTML = `<a href="#artists/${song.Artist}">${song.Artist}</a><br/><a href="#albums/${song.Album}">${song.Album}</a><br/>${song.Name}`;
      el("player-albumcover").innerHTML = song.Image.length > 0 ? `<img class="easeload" onload="this.style.opacity=1" src="${song.Image}">` : ``;
      if ("mediaSession" in navigator) {
        navigator.mediaSession.metadata = new MediaMetadata({
          title: song.Name,
          artist: song.Artist,
          album: song.Album,
          artwork: [{ src: song.Image ?? "" }]
        });
        navigator.mediaSession.setActionHandler("play", () => {
          audio.play();
        });
        navigator.mediaSession.setActionHandler("pause", () => {
          audio.pause();
        });
        navigator.mediaSession.setActionHandler("seekto", (details) => {
          if (details.seekTime) {
            audio.currentTime = details.seekTime;
          }
        });
        navigator.mediaSession.setActionHandler("previoustrack", () => prevtrack());
        navigator.mediaSession.setActionHandler("nexttrack", () => nexttrack());
      }
    } else {
      audio.pause();
      let songIds = [];
      for (let idx = playlistIdx; idx < playlist.length; idx++) {
        songIds.push(playlist[idx].SongId);
      }
      sonoscommand({ SongIDs: songIds });
    }
  }
  var prevApi = "";
  var cachedScrolls = {};
  function getmusic(api) {
    if (prevApi.length > 0) {
      cachedScrolls[prevApi] = el("results").scrollTop;
    }
    if (prevApi == "" && api == "" && window.innerWidth > 750) {
      api = "albums";
    }
    el("results").innerHTML = "";
    var req = new XMLHttpRequest();
    if (api.startsWith("search/")) {
      req.open("GET", "/api/" + api);
    } else {
      req.open("GET", "/api/music/" + api);
    }
    req.onload = function() {
      let res = JSON.parse(req.response);
      let html = "";
      if (api == "albums") {
        html += `<div class="albumcontainer">`;
        for (let result of res.Results) {
          html += `<div class="album"><a href="#${result.Link}">`;
          if (result.Image.length > 0) {
            html += `<div class="albumbox"><img class="albumbox easeload" onload="this.style.opacity=1" loading="lazy" src="/content${result.Image}" /></div>`;
          } else {
            html += `<div class="albumbox"></div>`;
          }
          html += `<div class="albumtext">${result.Name}<br/>${result.Artist}</div></a></div>`;
        }
        html += `</div>`;
      } else {
        let first = true;
        for (let idx in res.Results) {
          let result = res.Results[idx];
          if (result.Type == "Song") {
            html += `<div class="song ${first ? "firstpad" : ""}">
                        <a class="play" data-idx="${idx}">${result.Name}</a>
                    </div>`;
          } else if (result.Type == "AlbumHeader") {
            html += `<div class="albumheader ${first ? "" : "albumheaderpad"}"><div>`;
            if (result.Image.length > 0) {
              html += `<div class="albumbox"><img class="albumbox easeload" onload="this.style.opacity=1" loading="lazy" src="/content${result.Image}" /></div>`;
            } else {
              html += `<div class="albumbox"></div>`;
            }
            html += `</div><div><h1>${result.Name}</h1><a href="#artists/${result.Artist}">${result.Artist}</a></div></div>`;
          } else {
            let icon = "folder";
            if (result.Type == "Artist" || result.Name == "Artists") {
              icon = "person";
            } else if (result.Name == "Albums") {
              icon = "album";
            } else if (result.Name == "Songs") {
              icon = "music_note";
            }
            html += `<div class="folder ${first ? "firstpad" : ""}"><a href="#${result.Link}"><i class="material-icons">${icon}</i><span>${result.Name}</span></a></div>`;
          }
          first = false;
        }
      }
      el("results").innerHTML = html;
      for (let node of document.querySelectorAll(".play")) {
        let idx = parseInt(node.dataset.idx);
        node.onclick = function() {
          playlist = [];
          for (let rx in res.Results) {
            let ridx = parseInt(rx);
            if (res.Results[ridx].Type == "Song") {
              if (idx == ridx) {
                playlistIdx = playlist.length;
              }
              playlist.push(res.Results[ridx]);
            }
          }
          playsong();
        };
      }
      if (cachedScrolls[api]) {
        el("results").scrollTop = cachedScrolls[api];
      }
      prevApi = api;
    };
    req.send();
  }
  var sonosRooms = [];
  function sonosroomhtml(sonosRoom2) {
    let html = `<div onclick="setspeaker(this)" data-room="" class="${sonosRoom2.length == 0 ? "selected" : ""}"><span class="valign-wrapper"><i class="material-icons">${sonosRoom2.length == 0 ? "check" : "speaker"}</i> Speaker</span></div>`;
    for (let room of sonosRooms) {
      html += `<div onclick="setspeaker(this)" data-room="${room}" class="${sonosRoom2 == room ? "selected" : ""}"><span class="valign-wrapper"><i class="material-icons">${sonosRoom2 == room ? "check" : "speaker"}</i> ${room}</span></div>`;
    }
    return html;
  }
  var evts = null;
  var sonosTimeSecs = 0;
  var sonosTickId = 0;
  function tickSonosTime() {
    if (sonosRoom.length == 0 || !is_playing) {
      clearInterval(sonosTickId);
      return;
    }
    el("player-curtime").innerText = formatTime(sonosTimeSecs);
    if (enable_range_update) {
      el("player-range").value = sonosTimeSecs.toString();
    }
    sonosTimeSecs++;
  }
  window.setspeaker = function(elem) {
    setspeaker(elem.dataset.room);
  };
  function setspeaker(room) {
    audio.pause();
    el("sonos-list").innerHTML = sonosroomhtml(room);
    if (evts != null) {
      evts.close();
    }
    if (room.length > 0) {
      console.log("connecting " + room);
      let first_message = true;
      el("player-right").classList.remove("player-right-volume");
      evts = new EventSource("/api/sonos/" + room + "/events");
      evts.onmessage = (event) => {
        if (first_message) {
          first_message = false;
          sonosRoom = room;
          console.log("set " + sonosRoom);
          el("player-speakers").innerHTML = `<span class="valign-wrapper"><i class="material-icons selected">speaker</i>${sonosRoom}</span>`;
        }
        let res = JSON.parse(event.data);
        console.log(res);
        if (res.Sonos.Track) {
          is_playing = res.Sonos.Playing;
          el("player-play").innerHTML = res.Sonos.Playing ? `<i class="material-icons">pause</i>` : `<i class="material-icons">play_arrow</i>`;
          el("player-endtime").innerText = formatTime(parseTime(res.Sonos.Duration));
          el("player-range").max = parseTime(res.Sonos.Duration).toString();
          sonosTimeSecs = parseTime(res.Sonos.Position);
          tickSonosTime();
          if (is_playing) {
            clearInterval(sonosTickId);
            sonosTickId = setInterval(tickSonosTime, 1e3);
          }
          el("player-info").innerHTML = `<a href="#artists/${res.Sonos.Artist ?? ""}">${res.Sonos.Artist ?? ""}</a><br/><a href="#albums/${res.Sonos.Album ?? ""}">${res.Sonos.Album ?? ""}</a><br/>${res.Sonos.Track ?? ""}`;
          let newArt = res.Sonos.AlbumArtURI ? `<img class="easeload" onload="this.style.opacity=1" src="${res.Sonos.AlbumArtURI}">` : ``;
          if (el("player-albumcover").innerHTML != newArt) {
            el("player-albumcover").innerHTML = newArt;
          }
        }
        if (res.Sonos.Volume && enable_volume_update) {
          el("player-volume").value = res.Sonos.Volume.toString();
        }
      };
      evts.onerror = () => {
        console.log("connection lost");
        setspeaker("");
      };
    } else {
      el("player-speakers").innerHTML = `<span class="valign-wrapper"><i class="material-icons">speaker</i></span>`;
      el("player-right").classList.add("player-right-volume");
      el("player-albumcover").innerHTML = "";
      el("player-info").innerHTML = "";
      el("player-range").max = "1";
      el("player-range").value = "0";
      el("player-endtime").innerText = "0:00";
      el("player-curtime").innerText = "0:00";
    }
  }
  function refreshsonos() {
    var req = new XMLHttpRequest();
    req.open("GET", "/api/sonos");
    req.onload = function() {
      sonosRooms = JSON.parse(req.response).Rooms ?? [];
      el("sonos-list").innerHTML = sonosroomhtml(sonosRoom);
      el("player-speakers").onclick = (e) => {
        let style = el("sonos-list").style;
        let button = el("player-speakers").getBoundingClientRect();
        style.left = Math.min(window.innerWidth - 210, button.x) + "px";
        style.bottom = window.innerHeight - button.y + 15 + "px";
        style.display = "block";
        e.stopPropagation();
      };
      document.body.onclick = function() {
        el("sonos-list").style.display = "none";
      };
    };
    el("sonos-list").innerHTML = "";
    req.send();
  }
  window.onhashchange = function() {
    getmusic(window.location.hash.slice(1));
  };
  window.onload = function() {
    el("search").focus();
    el("search").oninput = function() {
      let searchstr = el("search").value;
      if (!window.location.hash.slice(1).startsWith("search")) {
        window.location.hash = "search/" + searchstr;
      } else {
        getmusic("search/" + searchstr);
      }
    };
    getmusic(window.location.hash.slice(1));
    refreshsonos();
    el("player-play").onclick = function() {
      if (is_playing) {
        if (sonosRoom.length > 0) {
          sonoscommand({ Action: "Pause" });
        } else {
          audio.pause();
        }
      } else {
        if (sonosRoom.length > 0) {
          sonoscommand({ Action: "Play" });
        } else {
          audio.play();
        }
      }
    };
    el("player-range").onmousedown = function() {
      enable_range_update = false;
    };
    el("player-range").onmouseleave = function() {
      enable_range_update = true;
    };
    el("player-range").oninput = function() {
      enable_range_update = true;
      let time = parseFloat(el("player-range").value);
      if (sonosRoom.length > 0) {
        sonoscommand({ SetTimeSecs: time });
      } else {
        audio.currentTime = time;
      }
    };
    el("player-mute").onclick = function() {
      if (audio.volume == 0) {
        audio.volume = unmute_volume;
      } else {
        unmute_volume = audio.volume;
        audio.volume = 0;
      }
    };
    el("player-volume").onmousedown = function() {
      enable_volume_update = false;
    };
    el("player-volume").onmouseleave = function() {
      enable_volume_update = true;
    };
    el("player-volume").oninput = function() {
      enable_volume_update = true;
      let volume = parseFloat(el("player-volume").value);
      audio.volume = volume / 100;
      if (sonosRoom.length > 0) {
        sonoscommand({ Volume: volume });
      }
    };
    el("player-prev").onclick = function() {
      prevtrack();
    };
    el("player-next").onclick = function() {
      nexttrack();
    };
  };
})();
//# sourceMappingURL=music.js.map
