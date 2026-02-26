package rtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	// ErrCodecNotFound is returned when a requested codec is not available
	ErrCodecNotFound = errors.New("codec not found")
	// ErrCodecNegotiationFailed is returned when codec negotiation fails
	ErrCodecNegotiationFailed = errors.New("codec negotiation failed")
)

// Codec types
const (
	CodecOpus    = "opus"
	CodecPCMU    = "PCMU"
	CodecPCMA    = "PCMA"
	CodecVP8     = "VP8"
	CodecVP9     = "VP9"
	CodecH264    = "H264"
	CodecAV1     = "AV1"
)

// Media types
const (
	MediaAudio = "audio"
	MediaVideo = "video"
)

// Codec represents a media codec with its parameters
type Codec struct {
	// Name is the codec name (e.g., "opus", "VP8")
	Name string `json:"name"`
	// Type is the media type ("audio" or "video")
	Type string `json:"type"`
	// ClockRate is the codec clock rate in Hz
	ClockRate int `json:"clock_rate"`
	// Channels is the number of audio channels (for audio codecs)
	Channels int `json:"channels,omitempty"`
	// PayloadType is the RTP payload type number
	PayloadType uint8 `json:"payload_type"`
	// Parameters contains codec-specific parameters
	Parameters map[string]string `json:"parameters,omitempty"`
	// Priority is the codec priority (higher = preferred)
	Priority int `json:"priority"`
	// Required indicates if this codec must be supported
	Required bool `json:"required"`
}

// CodecCapabilities represents supported codecs and their configurations
type CodecCapabilities struct {
	AudioCodecs []*Codec `json:"audio_codecs"`
	VideoCodecs []*Codec `json:"video_codecs"`
}

// DefaultCapabilities returns the default codec capabilities for Babylon Tower
func DefaultCapabilities() *CodecCapabilities {
	return &CodecCapabilities{
		AudioCodecs: []*Codec{
			{
				Name:        CodecOpus,
				Type:        MediaAudio,
				ClockRate:   48000,
				Channels:    2,
				PayloadType: 111,
				Parameters: map[string]string{
					"minptime":     "10",
					"useinbandfec": "1",
				},
				Priority: 100,
				Required: true, // MANDATORY per spec
			},
			{
				Name:        CodecPCMU,
				Type:        MediaAudio,
				ClockRate:   8000,
				Channels:    1,
				PayloadType: 0,
				Priority:    50,
				Required:    false,
			},
			{
				Name:        CodecPCMA,
				Type:        MediaAudio,
				ClockRate:   8000,
				Channels:    1,
				PayloadType: 8,
				Priority:    50,
				Required:    false,
			},
		},
		VideoCodecs: []*Codec{
			{
				Name:        CodecVP9,
				Type:        MediaVideo,
				ClockRate:   90000,
				PayloadType: 98,
				Parameters: map[string]string{
					"profile-id": "0",
				},
				Priority: 100,
				Required: false, // Preferred but not mandatory
			},
			{
				Name:        CodecVP8,
				Type:        MediaVideo,
				ClockRate:   90000,
				PayloadType: 96,
				Priority:    90,
				Required:    true, // MANDATORY baseline per spec
			},
			{
				Name:        CodecH264,
				Type:        MediaVideo,
				ClockRate:   90000,
				PayloadType: 102,
				Parameters: map[string]string{
					"profile-level-id": "42e01f",
					"level-asymmetry-allowed": "1",
					"packetization-mode": "1",
				},
				Priority: 70,
				Required: false,
			},
			{
				Name:        CodecAV1,
				Type:        MediaVideo,
				ClockRate:   90000,
				PayloadType: 99,
				Priority:    60,
				Required:    false, // Optional
			},
		},
	}
}

// SDP represents a simplified SDP offer/answer
type SDP struct {
	Version      int           `json:"version"`
	Origin       string        `json:"origin"`
	SessionName  string        `json:"session_name"`
	Connection   string        `json:"connection"`
	Timing       string        `json:"timing"`
	MediaSections []*MediaSection `json:"media_sections"`
	Attributes   []string      `json:"attributes,omitempty"`
}

// MediaSection represents an SDP media section (m= line)
type MediaSection struct {
	MediaType   string   `json:"media_type"`
	Port        int      `json:"port"`
	Protocol    string   `json:"protocol"`
	Formats     []string `json:"formats"`
	Attributes  []string `json:"attributes,omitempty"`
	Direction   string   `json:"direction,omitempty"` // sendrecv, sendonly, recvonly, inactive
	SSRC        uint32   `json:"ssrc,omitempty"`
	SSRCGroup   []uint32 `json:"ssrc_group,omitempty"`
}

// CodecNegotiator handles codec negotiation between peers
type CodecNegotiator struct {
	localCaps  *CodecCapabilities
	remoteCaps *CodecCapabilities
}

// NewCodecNegotiator creates a new codec negotiator
func NewCodecNegotiator() *CodecNegotiator {
	return &CodecNegotiator{
		localCaps: DefaultCapabilities(),
	}
}

// NegotiateAudio performs audio codec negotiation
func (n *CodecNegotiator) NegotiateAudio(remoteSDP string) (string, *Codec, error) {
	// Parse remote SDP
	remoteCodecs, err := n.parseAudioCodecs(remoteSDP)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse remote audio codecs: %w", err)
	}

	// Find best matching codec
	selectedCodec := n.selectBestAudioCodec(remoteCodecs)
	if selectedCodec == nil {
		return "", nil, ErrCodecNegotiationFailed
	}

	// Generate answer SDP
	answerSDP := n.generateAudioAnswer(selectedCodec)

	return answerSDP, selectedCodec, nil
}

// NegotiateVideo performs video codec negotiation
func (n *CodecNegotiator) NegotiateVideo(remoteSDP string) (string, *Codec, error) {
	// Parse remote SDP
	remoteCodecs, err := n.parseVideoCodecs(remoteSDP)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse remote video codecs: %w", err)
	}

	// Find best matching codec
	selectedCodec := n.selectBestVideoCodec(remoteCodecs)
	if selectedCodec == nil {
		return "", nil, ErrCodecNegotiationFailed
	}

	// Generate answer SDP
	answerSDP := n.generateVideoAnswer(selectedCodec)

	return answerSDP, selectedCodec, nil
}

// parseAudioCodecs extracts audio codecs from an SDP string
func (n *CodecNegotiator) parseAudioCodecs(sdp string) ([]*Codec, error) {
	var codecs []*Codec

	// Simple SDP parsing - in production, use a proper SDP parser
	lines := strings.Split(sdp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "a=rtpmap:") {
			codec := n.parseRTPMap(line)
			if codec != nil && codec.Type == MediaAudio {
				codecs = append(codecs, codec)
			}
		}
	}

	return codecs, nil
}

// parseVideoCodecs extracts video codecs from an SDP string
func (n *CodecNegotiator) parseVideoCodecs(sdp string) ([]*Codec, error) {
	var codecs []*Codec

	lines := strings.Split(sdp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "a=rtpmap:") {
			codec := n.parseRTPMap(line)
			if codec != nil && codec.Type == MediaVideo {
				codecs = append(codecs, codec)
			}
		}
	}

	return codecs, nil
}

// parseRTPMap parses an RTP map attribute
func (n *CodecNegotiator) parseRTPMap(line string) *Codec {
	// Format: a=rtpmap:<payload_type> <codec_name>/<clock_rate>[/<channels>]
	parts := strings.SplitN(strings.TrimPrefix(line, "a=rtpmap:"), " ", 2)
	if len(parts) != 2 {
		return nil
	}

	payloadType := parts[0]
	codecParts := strings.Split(parts[1], "/")
	if len(codecParts) < 2 {
		return nil
	}

	name := codecParts[0]
	var clockRate, channels int
	fmt.Sscanf(codecParts[1], "%d", &clockRate)
	if len(codecParts) >= 3 {
		fmt.Sscanf(codecParts[2], "%d", &channels)
	}

	mediaType := MediaAudio
	if isVideoCodec(name) {
		mediaType = MediaVideo
	}

	return &Codec{
		Name:        name,
		Type:        mediaType,
		ClockRate:   clockRate,
		Channels:    channels,
		PayloadType: parseUint8(payloadType),
	}
}

// selectBestAudioCodec selects the best audio codec from available options
func (n *CodecNegotiator) selectBestAudioCodec(remoteCodecs []*Codec) *Codec {
	// Sort local codecs by priority
	localCodecs := make([]*Codec, len(n.localCaps.AudioCodecs))
	copy(localCodecs, n.localCaps.AudioCodecs)
	sort.Slice(localCodecs, func(i, j int) bool {
		return localCodecs[i].Priority > localCodecs[j].Priority
	})

	// Find first matching codec
	for _, localCodec := range localCodecs {
		for _, remoteCodec := range remoteCodecs {
			if strings.EqualFold(localCodec.Name, remoteCodec.Name) {
				// Use local codec parameters
				result := *localCodec
				result.PayloadType = remoteCodec.PayloadType
				return &result
			}
		}
	}

	return nil
}

// selectBestVideoCodec selects the best video codec from available options
func (n *CodecNegotiator) selectBestVideoCodec(remoteCodecs []*Codec) *Codec {
	// Sort local codecs by priority
	localCodecs := make([]*Codec, len(n.localCaps.VideoCodecs))
	copy(localCodecs, n.localCaps.VideoCodecs)
	sort.Slice(localCodecs, func(i, j int) bool {
		return localCodecs[i].Priority > localCodecs[j].Priority
	})

	// Find first matching codec
	for _, localCodec := range localCodecs {
		for _, remoteCodec := range remoteCodecs {
			if strings.EqualFold(localCodec.Name, remoteCodec.Name) {
				// Use local codec parameters
				result := *localCodec
				result.PayloadType = remoteCodec.PayloadType
				return &result
			}
		}
	}

	return nil
}

// generateAudioAnswer generates an SDP answer for audio
func (n *CodecNegotiator) generateAudioAnswer(codec *Codec) string {
	sdp := &SDP{
		Version:     0,
		Origin:      "- 0 0 IN IP4 127.0.0.1",
		SessionName: "Babylon Tower Audio",
		Connection:  "IN IP4 127.0.0.1",
		Timing:      "0 0",
		MediaSections: []*MediaSection{
			{
				MediaType: "audio",
				Port:      9,
				Protocol:  "UDP/TLS/RTP/SAVPF",
				Formats:   []string{fmt.Sprintf("%d", codec.PayloadType)},
				Attributes: []string{
					fmt.Sprintf("rtpmap:%d %s/%d/%d", codec.PayloadType, codec.Name, codec.ClockRate, codec.Channels),
				},
				Direction: "sendrecv",
			},
		},
	}

	return sdp.Marshal()
}

// generateVideoAnswer generates an SDP answer for video
func (n *CodecNegotiator) generateVideoAnswer(codec *Codec) string {
	sdp := &SDP{
		Version:     0,
		Origin:      "- 0 0 IN IP4 127.0.0.1",
		SessionName: "Babylon Tower Video",
		Connection:  "IN IP4 127.0.0.1",
		Timing:      "0 0",
		MediaSections: []*MediaSection{
			{
				MediaType: "video",
				Port:      9,
				Protocol:  "UDP/TLS/RTP/SAVPF",
				Formats:   []string{fmt.Sprintf("%d", codec.PayloadType)},
				Attributes: []string{
					fmt.Sprintf("rtpmap:%d %s/%d", codec.PayloadType, codec.Name, codec.ClockRate),
				},
				Direction: "sendrecv",
			},
		},
	}

	return sdp.Marshal()
}

// Marshal serializes SDP to a string
func (s *SDP) Marshal() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("v=%d\n", s.Version))
	builder.WriteString(fmt.Sprintf("o=%s\n", s.Origin))
	builder.WriteString(fmt.Sprintf("s=%s\n", s.SessionName))
	builder.WriteString(fmt.Sprintf("c=%s\n", s.Connection))
	builder.WriteString(fmt.Sprintf("t=%s\n", s.Timing))

	for _, attr := range s.Attributes {
		builder.WriteString(fmt.Sprintf("a=%s\n", attr))
	}

	for _, media := range s.MediaSections {
		formats := strings.Join(media.Formats, " ")
		builder.WriteString(fmt.Sprintf("m=%s %d %s %s\n",
			media.MediaType, media.Port, media.Protocol, formats))

		for _, attr := range media.Attributes {
			builder.WriteString(fmt.Sprintf("a=%s\n", attr))
		}

		if media.Direction != "" {
			builder.WriteString(fmt.Sprintf("a=%s\n", media.Direction))
		}
	}

	return strings.TrimSpace(builder.String())
}

// GetSupportedAudioCodecs returns a list of supported audio codec names
func (n *CodecNegotiator) GetSupportedAudioCodecs() []string {
	var names []string
	for _, codec := range n.localCaps.AudioCodecs {
		names = append(names, codec.Name)
	}
	return names
}

// GetSupportedVideoCodecs returns a list of supported video codec names
func (n *CodecNegotiator) GetSupportedVideoCodecs() []string {
	var names []string
	for _, codec := range n.localCaps.VideoCodecs {
		names = append(names, codec.Name)
	}
	return names
}

// GetCodecParameters returns the parameters for a specific codec
func (n *CodecNegotiator) GetCodecParameters(name string) (map[string]string, error) {
	name = strings.ToLower(name)

	// Search audio codecs
	for _, codec := range n.localCaps.AudioCodecs {
		if strings.EqualFold(codec.Name, name) {
			return codec.Parameters, nil
		}
	}

	// Search video codecs
	for _, codec := range n.localCaps.VideoCodecs {
		if strings.EqualFold(codec.Name, name) {
			return codec.Parameters, nil
		}
	}

	return nil, ErrCodecNotFound
}

// isVideoCodec checks if a codec name is a video codec
func isVideoCodec(name string) bool {
	videoCodecs := []string{CodecVP8, CodecVP9, CodecH264, CodecAV1}
	name = strings.ToUpper(name)
	for _, vc := range videoCodecs {
		if name == strings.ToUpper(vc) {
			return true
		}
	}
	return false
}

// parseUint8 parses a string to uint8
func parseUint8(s string) uint8 {
	var v uint8
	fmt.Sscanf(s, "%d", &v)
	return v
}

// ToJSON serializes codec capabilities to JSON
func (c *CodecCapabilities) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}

// FromJSON deserializes codec capabilities from JSON
func FromJSON(data []byte) (*CodecCapabilities, error) {
	var caps CodecCapabilities
	if err := json.Unmarshal(data, &caps); err != nil {
		return nil, err
	}
	return &caps, nil
}
