package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/pion/mediadevices/pkg/log"
	"net/http"

	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/mediadevices/pkg/codec/vpx"       // This is required to use VP8/VP9 video encoder
	_ "github.com/pion/mediadevices/pkg/driver/screen" // This is required to register screen capture adapter
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/webrtc/v2"
)

func main() {
	addr := flag.String("address", "127.0.0.1:50000", "Address that the HTTP server is hosted on.")
	flag.Parse()
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}



	// Create a new RTCPeerConnection
	mediaEngine := webrtc.MediaEngine{}
	mediaEngine.RegisterDefaultCodecs()

	setttingEngine := webrtc.SettingEngine{}
	setttingEngine.LoggerFactory = log.CustomLoggerFactory{}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine), webrtc.WithSettingEngine(setttingEngine))
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		panic(err)
	}

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	})

	md := mediadevices.NewMediaDevices(peerConnection)

	vp8Params, err := vpx.NewVP8Params()
	if err != nil {
		panic(err)
	}
	vp8Params.BitRate = 100000 // 100kbps

	s, err := md.GetDisplayMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.Enabled = true
			c.VideoTransform = video.Scale(-1, 360, nil) // Resize to 360p
			c.VideoEncoderBuilders = []codec.VideoEncoderBuilder{&vp8Params}
		},
	})
	if err != nil {
		panic(err)
	}

	for _, tracker := range s.GetTracks() {
		t := tracker.Track()
		tracker.OnEnded(func(err error) {
			fmt.Printf("Track (ID: %s, Label: %s) ended with error: %v\n",
				t.ID(), t.Label(), err)
		})
		_, err = peerConnection.AddTransceiverFromTrack(t,
			webrtc.RtpTransceiverInit{
				Direction: webrtc.RTPTransceiverDirectionSendonly,
			},
		)
		if err != nil {
			panic(err)
		}
	}

	// Create an offer
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\noriginal offer: %+v\n", offer)

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		panic(err)
	}

	// Exchange the offer for the answer
	answer := mustSignalViaHTTP(offer, *addr)
	fmt.Printf("answer: %+v\n", answer)

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(answer)
	if err != nil {
		panic(err)
	}

	select {}
}

// mustSignalViaHTTP exchange the SDP offer and answer using an HTTP Post request.
func mustSignalViaHTTP(offer webrtc.SessionDescription, address string) webrtc.SessionDescription {
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(offer)
	if err != nil {
		panic(err)
	}

	resp, err := http.Post("http://"+address, "application/json; charset=utf-8", b)
	if err != nil {
		panic(err)
	}
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			panic(closeErr)
		}
	}()

	var answer webrtc.SessionDescription
	err = json.NewDecoder(resp.Body).Decode(&answer)
	if err != nil {
		panic(err)
	}

	return answer
}