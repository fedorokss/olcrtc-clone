package videochannel

import (
	"errors"
	"strings"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
)

var ErrUnexpectedFrameSize = errors.New("unexpected encoder frame size")

type codecSpec struct {
	mimeType     string
	capability   webrtc.RTPCodecCapability
	depacketizer func() rtp.Depacketizer
}

var (
	specH264 = codecSpec{
		mimeType: webrtc.MimeTypeH264,
		capability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeH264,
			ClockRate: 90000,
		},
		depacketizer: func() rtp.Depacketizer { return &codecs.H264Packet{} },
	}
	specVP9 = codecSpec{
		mimeType: webrtc.MimeTypeVP9,
		capability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP9,
			ClockRate: 90000,
		},
		depacketizer: func() rtp.Depacketizer { return &codecs.VP9Packet{} },
	}
	specVP8 = codecSpec{
		mimeType: webrtc.MimeTypeVP8,
		capability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		depacketizer: func() rtp.Depacketizer { return &codecs.VP8Packet{} },
	}
)

var codecSpecsByMime = map[string]codecSpec{
	strings.ToLower(webrtc.MimeTypeH264): specH264,
	strings.ToLower(webrtc.MimeTypeVP9):  specVP9,
	strings.ToLower(webrtc.MimeTypeVP8):  specVP8,
}

func codecSpecForCarrier(_ string) codecSpec {
	return specVP8
}

func codecSpecForMime(mimeType string) (codecSpec, bool) {
	spec, ok := codecSpecsByMime[strings.ToLower(mimeType)]
	return spec, ok
}

func h264CodecSpec() codecSpec {
	return specH264
}

func vp9CodecSpec() codecSpec {
	return specVP9
}

func vp8CodecSpec() codecSpec {
	return specVP8
}
