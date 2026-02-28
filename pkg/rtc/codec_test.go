package rtc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCapabilities(t *testing.T) {
	caps := DefaultCapabilities()

	// Assert audio codecs
	require.NotEmpty(t, caps.AudioCodecs)
	assert.True(t, len(caps.AudioCodecs) >= 1)

	// Opus should be first and required
	opus := caps.AudioCodecs[0]
	assert.Equal(t, CodecOpus, opus.Name)
	assert.Equal(t, MediaAudio, opus.Type)
	assert.Equal(t, 48000, opus.ClockRate)
	assert.Equal(t, 2, opus.Channels)
	assert.Equal(t, uint8(111), opus.PayloadType)
	assert.True(t, opus.Required)
	assert.Equal(t, 100, opus.Priority)

	// Assert video codecs
	require.NotEmpty(t, caps.VideoCodecs)
	assert.True(t, len(caps.VideoCodecs) >= 1)

	// VP8 should be present and required
	var vp8 *Codec
	for _, codec := range caps.VideoCodecs {
		if codec.Name == CodecVP8 {
			vp8 = codec
			break
		}
	}
	require.NotNil(t, vp8)
	assert.Equal(t, MediaVideo, vp8.Type)
	assert.Equal(t, 90000, vp8.ClockRate)
	assert.True(t, vp8.Required)
}

func TestCodecNegotiator_NegotiateAudio(t *testing.T) {
	negotiator := NewCodecNegotiator()

	// Remote SDP with multiple codecs
	remoteSDP := `v=0
o=- 0 0 IN IP4 127.0.0.1
s=Test
m=audio 9 UDP/TLS/RTP/SAVPF 111 0 8
a=rtpmap:111 opus/48000/2
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
`

	answerSDP, codec, err := negotiator.NegotiateAudio(remoteSDP)

	require.NoError(t, err)
	assert.NotNil(t, codec)
	assert.Equal(t, CodecOpus, codec.Name)
	assert.Contains(t, answerSDP, "opus")
}

func TestCodecNegotiator_NegotiateVideo(t *testing.T) {
	negotiator := NewCodecNegotiator()

	// Remote SDP with video codecs
	remoteSDP := `v=0
o=- 0 0 IN IP4 127.0.0.1
s=Test
m=video 9 UDP/TLS/RTP/SAVPF 96 98 102
a=rtpmap:96 VP8/90000
a=rtpmap:98 VP9/90000
a=rtpmap:102 H264/90000
`

	answerSDP, codec, err := negotiator.NegotiateVideo(remoteSDP)

	require.NoError(t, err)
	assert.NotNil(t, codec)
	// VP9 has higher priority than VP8 in our config
	assert.Contains(t, []string{CodecVP9, CodecVP8}, codec.Name)
	assert.Contains(t, answerSDP, codec.Name)
}

func TestCodecNegotiator_NegotiateAudioNoMatch(t *testing.T) {
	negotiator := NewCodecNegotiator()

	// Remote SDP with unsupported codec
	remoteSDP := `v=0
o=- 0 0 IN IP4 127.0.0.1
s=Test
m=audio 9 UDP/TLS/RTP/SAVPF 99
a=rtpmap:99 UNSUPPORTED_CODEC/8000
`

	_, _, err := negotiator.NegotiateAudio(remoteSDP)

	assert.Error(t, err)
	assert.Equal(t, ErrCodecNegotiationFailed, err)
}

func TestCodecNegotiator_ParseRTPMap(t *testing.T) {
	negotiator := NewCodecNegotiator()

	tests := []struct {
		line      string
		wantName  string
		wantRate  int
		wantChans int
	}{
		{"a=rtpmap:111 opus/48000/2", "opus", 48000, 2},
		{"a=rtpmap:0 PCMU/8000", "PCMU", 8000, 0},
		{"a=rtpmap:96 VP8/90000", "VP8", 90000, 0},
		{"a=rtpmap:98 VP9/90000/1", "VP9", 90000, 1},
	}

	for _, tt := range tests {
		codec := negotiator.parseRTPMap(tt.line)
		require.NotNil(t, codec)
		assert.Equal(t, tt.wantName, codec.Name)
		assert.Equal(t, tt.wantRate, codec.ClockRate)
		assert.Equal(t, tt.wantChans, codec.Channels)
	}
}

func TestCodecNegotiator_GetSupportedCodecs(t *testing.T) {
	negotiator := NewCodecNegotiator()

	audioCodecs := negotiator.GetSupportedAudioCodecs()
	videoCodecs := negotiator.GetSupportedVideoCodecs()

	// Check audio codecs
	assert.Contains(t, audioCodecs, CodecOpus)
	assert.Contains(t, audioCodecs, CodecPCMU)
	assert.Contains(t, audioCodecs, CodecPCMA)

	// Check video codecs
	assert.Contains(t, videoCodecs, CodecVP8)
	assert.Contains(t, videoCodecs, CodecVP9)
	assert.Contains(t, videoCodecs, CodecH264)
}

func TestCodecNegotiator_GetCodecParameters(t *testing.T) {
	negotiator := NewCodecNegotiator()

	// Test Opus parameters
	params, err := negotiator.GetCodecParameters(CodecOpus)
	require.NoError(t, err)
	assert.NotEmpty(t, params)
	assert.Equal(t, "10", params["minptime"])
	assert.Equal(t, "1", params["useinbandfec"])

	// Test VP9 parameters
	params, err = negotiator.GetCodecParameters(CodecVP9)
	require.NoError(t, err)
	assert.Equal(t, "0", params["profile-id"])

	// Test unknown codec
	_, err = negotiator.GetCodecParameters("UNKNOWN_CODEC")
	assert.Error(t, err)
	assert.Equal(t, ErrCodecNotFound, err)
}

func TestSDP_Marshal(t *testing.T) {
	sdp := &SDP{
		Version:     0,
		Origin:      "- 0 0 IN IP4 127.0.0.1",
		SessionName: "Test Session",
		Connection:  "IN IP4 127.0.0.1",
		Timing:      "0 0",
		MediaSections: []*MediaSection{
			{
				MediaType: "audio",
				Port:      9,
				Protocol:  "UDP/TLS/RTP/SAVPF",
				Formats:   []string{"111", "0", "8"},
				Attributes: []string{
					"rtpmap:111 opus/48000/2",
					"rtpmap:0 PCMU/8000",
					"rtpmap:8 PCMA/8000",
				},
				Direction: "sendrecv",
			},
		},
	}

	marshaled := sdp.Marshal()

	assert.Contains(t, marshaled, "v=0")
	assert.Contains(t, marshaled, "o=- 0 0 IN IP4 127.0.0.1")
	assert.Contains(t, marshaled, "s=Test Session")
	assert.Contains(t, marshaled, "m=audio 9 UDP/TLS/RTP/SAVPF 111 0 8")
	assert.Contains(t, marshaled, "a=rtpmap:111 opus/48000/2")
	assert.Contains(t, marshaled, "a=sendrecv")
}

func TestSDP_MarshalEmpty(t *testing.T) {
	sdp := &SDP{
		Version:     0,
		Origin:      "- 0 0 IN IP4 127.0.0.1",
		SessionName: "Empty Session",
		Connection:  "IN IP4 127.0.0.1",
		Timing:      "0 0",
	}

	marshaled := sdp.Marshal()

	assert.Contains(t, marshaled, "v=0")
	assert.Contains(t, marshaled, "s=Empty Session")
	// Should not have media sections
	assert.NotContains(t, marshaled, "m=")
}

func TestIsVideoCodec(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"VP8", true},
		{"vp8", true},
		{"VP9", true},
		{"H264", true},
		{"AV1", true},
		{"opus", false},
		{"PCMU", false},
		{"UNKNOWN", false},
	}

	for _, tt := range tests {
		got := isVideoCodec(tt.name)
		assert.Equal(t, tt.want, got, "isVideoCodec(%q)", tt.name)
	}
}

func TestCodecCapabilities_JSON(t *testing.T) {
	caps := DefaultCapabilities()

	// Marshal to JSON
	data, err := caps.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Unmarshal from JSON
	restored, err := FromJSON(data)
	require.NoError(t, err)

	// Compare
	assert.Len(t, restored.AudioCodecs, len(caps.AudioCodecs))
	assert.Len(t, restored.VideoCodecs, len(caps.VideoCodecs))

	// Check first audio codec
	if len(caps.AudioCodecs) > 0 {
		assert.Equal(t, caps.AudioCodecs[0].Name, restored.AudioCodecs[0].Name)
		assert.Equal(t, caps.AudioCodecs[0].Priority, restored.AudioCodecs[0].Priority)
	}
}

func TestCodecNegotiator_SelectBestCodec(t *testing.T) {
	negotiator := NewCodecNegotiator()

	// Create remote codecs with different priorities
	remoteCodecs := []*Codec{
		{Name: "PCMU", Type: MediaAudio, ClockRate: 8000, Priority: 50},
		{Name: "opus", Type: MediaAudio, ClockRate: 48000, Priority: 80},
	}

	// Should select Opus (highest priority match)
	selected := negotiator.selectBestAudioCodec(remoteCodecs)
	require.NotNil(t, selected)
	assert.Equal(t, CodecOpus, selected.Name)
}

func TestCodecNegotiator_PayloadTypePreservation(t *testing.T) {
	negotiator := NewCodecNegotiator()

	// Remote SDP with non-standard payload type for Opus
	remoteSDP := `v=0
o=- 0 0 IN IP4 127.0.0.1
s=Test
m=audio 9 UDP/TLS/RTP/SAVPF 96
a=rtpmap:96 opus/48000/2
`

	_, codec, err := negotiator.NegotiateAudio(remoteSDP)
	require.NoError(t, err)

	// Payload type should be preserved from remote SDP
	assert.Equal(t, uint8(96), codec.PayloadType)
}

func TestGenerateSDPOffer(t *testing.T) {
	// This tests the placeholder SDP generation in CallManager
	cm := &CallManager{
		config: DefaultCallConfig(),
	}

	// Test audio offer
	audioSDP := cm.generateSDPOffer(CallTypeAudio)
	assert.Contains(t, audioSDP, "opus/48000/2")
	assert.Contains(t, audioSDP, "VP8/90000")
	assert.Contains(t, audioSDP, "Babylon Tower audio Call")

	// Test video offer
	videoSDP := cm.generateSDPOffer(CallTypeVideo)
	assert.Contains(t, videoSDP, "Babylon Tower video Call")
}

func TestGenerateSDPAnswer(t *testing.T) {
	cm := &CallManager{
		config: DefaultCallConfig(),
	}

	remoteSDP := `v=0
o=- 0 0 IN IP4 127.0.0.1
s=Remote
m=audio 9 UDP/TLS/RTP/SAVPF 111
a=rtpmap:111 opus/48000/2
`

	answerSDP := cm.generateSDPAnswer(CallTypeAudio, remoteSDP)
	assert.Contains(t, answerSDP, "opus/48000/2")
	assert.Contains(t, answerSDP, "Answer")
}

func TestCodecPriorityOrder(t *testing.T) {
	caps := DefaultCapabilities()

	// Verify audio codecs are ordered by priority
	for i := 1; i < len(caps.AudioCodecs); i++ {
		assert.LessOrEqual(t, caps.AudioCodecs[i].Priority, caps.AudioCodecs[i-1].Priority)
	}

	// Verify video codecs are ordered by priority
	for i := 1; i < len(caps.VideoCodecs); i++ {
		assert.LessOrEqual(t, caps.VideoCodecs[i].Priority, caps.VideoCodecs[i-1].Priority)
	}
}

func TestCodecRequiredFlag(t *testing.T) {
	caps := DefaultCapabilities()

	// Find required codecs
	var requiredAudio, requiredVideo []string

	for _, codec := range caps.AudioCodecs {
		if codec.Required {
			requiredAudio = append(requiredAudio, codec.Name)
		}
	}

	for _, codec := range caps.VideoCodecs {
		if codec.Required {
			requiredVideo = append(requiredVideo, codec.Name)
		}
	}

	// Per spec, Opus and VP8 are mandatory
	assert.Contains(t, requiredAudio, CodecOpus)
	assert.Contains(t, requiredVideo, CodecVP8)
}
