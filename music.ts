function el(id: string): HTMLElement { return document.getElementById(id) as any; }

type Result = {
    Name: string
    Type: "Song" | "Artist" | "Album" | "AlbumHeader" | "Folder"
    Link: string, Audio: string,
    Artist: string, Album: string, Image: string,
}

type ListMusicResult = {
    Results: Result[], NextPage: string, PrevPage: string
};

let audio = document.createElement("audio");
let nextaudio = document.createElement("audio");
let is_playing = false, enable_range_update = true, enable_volume_update = true;
let unmute_volume = audio.volume;
audio.onplay = function () {
    el("player-play").innerHTML = `<i class="material-icons">pause_arrow</i>`;
    is_playing = true;
};
audio.onpause = function () {
    el("player-play").innerHTML = `<i class="material-icons">play_arrow</i>`;
    is_playing = false;
};
function formatTime(time: number): string {
    let mins = Math.floor(time / 60).toString();
    let secs = Math.floor(time % 60).toString();
    if (secs.length < 2) {
        secs = "0" + secs;
    }
    return mins + ":" + secs;
}
audio.ontimeupdate = function () {
    if (enable_range_update) {
        el("player-curtime").innerText = formatTime(audio.currentTime);
        (el("player-range") as HTMLInputElement).value = audio.currentTime.toString();
    } else {
        el("player-curtime").innerText = formatTime(parseFloat((el("player-range") as HTMLInputElement).value));
    }
    if (audio.currentTime >= audio.duration) {
        nexttrack();
    }
};
audio.onloadedmetadata = function () {
    el("player-endtime").innerText = formatTime(audio.duration);
    let range = (el("player-range") as HTMLInputElement);
    range.max = audio.duration.toString();
    range.value = "0";
    el("player-curtime").innerText = "0:00";
};
audio.onvolumechange = function () {
    console.log(audio.volume);
    (el("player-volume") as HTMLInputElement).value = (audio.volume * 100).toString();
    el("player-mute").innerHTML = `<i class="material-icons">${audio.volume > 0 ? 'volume_up' : 'volume_mute'}</i>`;
};

let playlist: Result[] = [];
let playlistIdx = 0;
function nexttrack() {
    if (playlistIdx < playlist.length) {
        playlistIdx++;
        playsong();
    }
}
function prevtrack() {
    if (playlistIdx > 0) {
        playlistIdx--;
        playsong();
    }
}
function playsong() {
    if (playlistIdx < 0 || playlist.length == 0 || playlistIdx >= playlist.length) {
        audio.pause();
        return;
    }
    let song = playlist[playlistIdx];
    console.log("play " + song.Audio);
    audio.src = song.Audio;
    audio.load();
    audio.play();

    el("player-info").innerHTML = `<a href="#artists/${song.Artist}">${song.Artist}</a><br/><a href="#albums/${song.Album}">${song.Album}</a><br/>${song.Name}`;
    el("player-albumcover").innerHTML = (song.Image as string).length > 0 ? `<img src="${song.Image}">` : ``;
    if ('mediaSession' in navigator) {
        navigator.mediaSession.metadata = new MediaMetadata({
            title: song.Name, artist: song.Artist, album: song.Album, artwork: [{ src: song.Image ?? "" }],
        });
        navigator.mediaSession.setActionHandler('play', () => { audio.play(); });
        navigator.mediaSession.setActionHandler('pause', () => { audio.pause(); });
        navigator.mediaSession.setActionHandler('seekto', (details) => { if (details.seekTime) { audio.currentTime = details.seekTime; } });
        navigator.mediaSession.setActionHandler('previoustrack', () => prevtrack());
        navigator.mediaSession.setActionHandler('nexttrack', () => nexttrack());
    }
}

let prevApi = "";
let cachedScrolls: Record<string, number> = {};

function getmusic(api: string) {
    if (prevApi.length > 0) {
        cachedScrolls[prevApi] = el("results").scrollTop;
    }
    if (prevApi == "" && api == "" && window.innerWidth > 750) {
        api = "albums";
    }
    var req = new XMLHttpRequest();
    req.open("GET", "/api/music/" + api);
    req.onload = function () {
        let res = JSON.parse(req.response) as ListMusicResult;
        let html = "";
        if (api == "albums") {
            html += `<div class="albumcontainer">`;
            for (let result of res.Results) {
                html += `<div class="album"><a href="#${result.Link}">`;
                if (result.Image.length > 0) {
                    html += `<img class="albumbox" loading="lazy" src="/content/${result.Image}" />`;
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
                    html += `<div class="song ${first ? 'firstpad' : ''}">
                        <a class="play" data-idx="${idx}">${result.Name}</a>
                    </div>`;
                } else if (result.Type == "AlbumHeader") {
                    html += `<div class="albumheader ${first ? '' : 'albumheaderpad'}"><div>`;
                    if (result.Image.length > 0) {
                        html += `<img class="albumbox" loading="lazy" src="/content/${result.Image}" />`;
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
                    html += `<div class="folder ${first ? 'firstpad' : ''}"><a href="#${result.Link}"><i class="material-icons">${icon}</i><span>${result.Name}</span></a></div>`;
                }
                first = false;
            }
        }
        el("results").innerHTML = html;
        function nexttrack() { }
        function prevtrack() { }
        for (let node of document.querySelectorAll(".play")) {
            let idx = parseInt((node as HTMLElement).dataset.idx as string);
            (node as HTMLElement).onclick = function () {
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
    el("results").innerHTML = "";
}

window.onhashchange = function () {
    getmusic(window.location.hash.slice(1));
};
window.onload = function () {
    getmusic(window.location.hash.slice(1));
    el("player-play").onclick = function () {
        if (is_playing) {
            audio.pause();
        } else {
            audio.play();
        }
    };
    el("player-range").onmousedown = function () {
        enable_range_update = false;
    };
    el("player-range").onmouseleave = function () {
        enable_range_update = true;
    };
    el("player-range").oninput = function () {
        enable_range_update = true;
        let time = parseFloat((el("player-range") as HTMLInputElement).value);
        audio.currentTime = time;
    };
    el("player-mute").onclick = function () {
        if (audio.volume == 0) {
            audio.volume = unmute_volume;
        } else {
            unmute_volume = audio.volume;
            audio.volume = 0;
        }
    };
    el("player-volume").onmousedown = function () {
        enable_volume_update = false;
    };
    el("player-volume").onmouseleave = function () {
        enable_volume_update = true;
    };
    el("player-volume").oninput = function () {
        enable_volume_update = true;
        audio.volume = parseFloat((el("player-volume") as HTMLInputElement).value) / 100;
    };
    el("player-prev").onclick = function () {
        prevtrack();
    };
    el("player-next").onclick = function () {
        nexttrack();
    };
};
