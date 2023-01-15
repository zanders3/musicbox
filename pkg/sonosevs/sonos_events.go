package sonosevs

import "encoding/xml"

type AudioTransportEvent struct {
	XMLName    xml.Name `xml:"Event"`
	Text       string   `xml:",chardata"`
	Xmlns      string   `xml:"xmlns,attr"`
	R          string   `xml:"r,attr"`
	InstanceID struct {
		Text           string `xml:",chardata"`
		Val            string `xml:"val,attr"`
		TransportState struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"TransportState"`
		CurrentPlayMode struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"CurrentPlayMode"`
		CurrentCrossfadeMode struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"CurrentCrossfadeMode"`
		NumberOfTracks struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"NumberOfTracks"`
		CurrentTrack struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"CurrentTrack"`
		CurrentSection struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"CurrentSection"`
		CurrentTrackURI struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"CurrentTrackURI"`
		CurrentTrackDuration struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"CurrentTrackDuration"`
		CurrentTrackMetaData struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"CurrentTrackMetaData"`
		NextTrackURI struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"NextTrackURI"`
		NextTrackMetaData struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"NextTrackMetaData"`
		EnqueuedTransportURI struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"EnqueuedTransportURI"`
		EnqueuedTransportURIMetaData struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"EnqueuedTransportURIMetaData"`
	} `xml:"InstanceID"`
}

type DIDLLite struct {
	XMLName xml.Name `xml:"DIDL-Lite"`
	Text    string   `xml:",chardata"`
	Dc      string   `xml:"dc,attr"`
	Upnp    string   `xml:"upnp,attr"`
	R       string   `xml:"r,attr"`
	Xmlns   string   `xml:"xmlns,attr"`
	Item    struct {
		Text       string `xml:",chardata"`
		ID         string `xml:"id,attr"`
		ParentID   string `xml:"parentID,attr"`
		Restricted string `xml:"restricted,attr"`
		Res        struct {
			Text         string `xml:",chardata"`
			ProtocolInfo string `xml:"protocolInfo,attr"`
			Duration     string `xml:"duration,attr"`
		} `xml:"res"`
		StreamContent       string `xml:"streamContent"`
		RadioShowMd         string `xml:"radioShowMd"`
		StreamInfo          string `xml:"streamInfo"`
		AlbumArtURI         string `xml:"albumArtURI"`
		Title               string `xml:"title"`
		Class               string `xml:"class"`
		Creator             string `xml:"creator"`
		Album               string `xml:"album"`
		OriginalTrackNumber string `xml:"originalTrackNumber"`
		AlbumArtist         string `xml:"albumArtist"`
	} `xml:"item"`
}

type RenderingControlEvent struct {
	XMLName    xml.Name `xml:"Event"`
	Text       string   `xml:",chardata"`
	Xmlns      string   `xml:"xmlns,attr"`
	InstanceID struct {
		Text   string `xml:",chardata"`
		Val    string `xml:"val,attr"`
		Volume []struct {
			Text    string `xml:",chardata"`
			Channel string `xml:"channel,attr"`
			Val     string `xml:"val,attr"`
		} `xml:"Volume"`
		Mute []struct {
			Text    string `xml:",chardata"`
			Channel string `xml:"channel,attr"`
			Val     string `xml:"val,attr"`
		} `xml:"Mute"`
		Bass struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"Bass"`
		Treble struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"Treble"`
		Loudness struct {
			Text    string `xml:",chardata"`
			Channel string `xml:"channel,attr"`
			Val     string `xml:"val,attr"`
		} `xml:"Loudness"`
		OutputFixed struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"OutputFixed"`
		SpeakerSize struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"SpeakerSize"`
		SubGain struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"SubGain"`
		SubCrossover struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"SubCrossover"`
		SubPolarity struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"SubPolarity"`
		SubEnabled struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"SubEnabled"`
		SonarEnabled struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"SonarEnabled"`
		SonarCalibrationAvailable struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"SonarCalibrationAvailable"`
		PresetNameList struct {
			Text string `xml:",chardata"`
			Val  string `xml:"val,attr"`
		} `xml:"PresetNameList"`
	} `xml:"InstanceID"`
}
