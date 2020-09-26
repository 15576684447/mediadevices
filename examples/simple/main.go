package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/log"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v2"
	"net/http"

	// This is required to use opus audio encoder
	"github.com/pion/mediadevices/pkg/codec/opus"

	// If you don't like vpx, you can also use x264 by importing as below
	// "github.com/pion/mediadevices/pkg/codec/x264" // This is required to use h264 video encoder
	// or you can also use openh264 for alternative h264 implementation
	// "github.com/pion/mediadevices/pkg/codec/openh264"
	"github.com/pion/mediadevices/pkg/codec/vpx" // This is required to use VP8/VP9 video encoder

	// Note: If you don't have a camera or microphone or your adapters are not supported,
	//       you can always swap your adapters with our dummy adapters below.
	// _ "github.com/pion/mediadevices/pkg/driver/videotest"
	// _ "github.com/pion/mediadevices/pkg/driver/audiotest"
	_ "github.com/pion/mediadevices/pkg/driver/camera"     // This is required to register camera adapter
	_ "github.com/pion/mediadevices/pkg/driver/microphone" // This is required to register microphone adapter
)

const (
	videoCodecName = webrtc.VP8
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

	opusParams, err := opus.NewParams()
	if err != nil {
		panic(err)
	}
	opusParams.BitRate = 32000 // 32kbps

	vp8Params, err := vpx.NewVP8Params()
	if err != nil {
		panic(err)
	}
	vp8Params.BitRate = 100000 // 100kbps
	//定义Video和Audio 约束函数
	s, err := md.GetUserMedia(mediadevices.MediaStreamConstraints{
		Audio: func(c *mediadevices.MediaTrackConstraints) {
			c.Enabled = true
			c.AudioEncoderBuilders = []codec.AudioEncoderBuilder{&opusParams}
		},
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.FrameFormat = prop.FrameFormat(frame.FormatYUY2)
			c.Enabled = true
			c.Width = prop.Int(640)
			c.Height = prop.Int(480)
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
		//增加各个track到pc
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