package rtcpeer

import (
	"context"
	"testing"

	"github.com/pion/webrtc/v4"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestParseOpusChannels(t *testing.T) {
	tests := []struct {
		name string
		sdp  string
		want int
	}{
		{
			name: "mono opus",
			sdp:  "a=rtpmap:111 opus/48000/1\r\n",
			want: 1,
		},
		{
			name: "stereo opus",
			sdp:  "v=0\nm=audio 9 UDP/TLS/RTP/SAVPF 111\na=rtpmap:111 opus/48000/2\n",
			want: 2,
		},
		{
			name: "defaults to mono without channel info",
			sdp:  "a=rtpmap:111 opus/48000\n",
			want: webrtcChannels,
		},
		{
			name: "defaults to mono for unsupported channels",
			sdp:  "a=rtpmap:111 opus/48000/6\n",
			want: webrtcChannels,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseOpusChannels(tt.sdp); got != tt.want {
				t.Fatalf("parseOpusChannels() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNormalizeClientID(t *testing.T) {
	if got := normalizeClientID(" client-1 "); got != "client-1" {
		t.Fatalf("normalizeClientID() = %q, want client-1", got)
	}
	if got := normalizeClientID(" "); got != "default" {
		t.Fatalf("normalizeClientID() = %q, want default", got)
	}
}

func TestHandleICEQueuesCandidateBeforePeerConnection(t *testing.T) {
	mid := "0"
	lineIndex := uint16(1)
	s := &stage{}

	s.handleICE(types.RTCSignal{
		Type:     "webrtc.ice",
		ClientID: " client-a ",
		Candidate: &types.RTCIceCandidate{
			Candidate:     "candidate:1 1 udp 1 127.0.0.1 50000 typ host",
			SDPMid:        &mid,
			SDPMLineIndex: &lineIndex,
		},
	})

	peer := s.getPeer("client-a")
	if peer == nil {
		t.Fatal("expected peer state to be created")
	}
	if len(peer.pendingICE) != 1 {
		t.Fatalf("expected 1 pending ICE candidate, got %d", len(peer.pendingICE))
	}
	got := peer.pendingICE[0]
	if got.Candidate != "candidate:1 1 udp 1 127.0.0.1 50000 typ host" {
		t.Fatalf("unexpected candidate: %q", got.Candidate)
	}
	if got.SDPMid == nil || *got.SDPMid != mid {
		t.Fatalf("unexpected SDPMid: %#v", got.SDPMid)
	}
	if got.SDPMLineIndex == nil || *got.SDPMLineIndex != lineIndex {
		t.Fatalf("unexpected SDPMLineIndex: %#v", got.SDPMLineIndex)
	}
}

func TestHandleICEIgnoresNilCandidate(t *testing.T) {
	s := &stage{}
	s.handleICE(types.RTCSignal{Type: "webrtc.ice", ClientID: "client-a"})

	if peer := s.getPeer("client-a"); peer != nil {
		t.Fatalf("expected no peer to be created, got %#v", peer)
	}
}

func TestResetPeerClearsConnectionState(t *testing.T) {
	s := &stage{
		ctx:        context.Background(),
		downstream: make(chan types.Event, 1),
	}
	peer := &peerState{
		id:           "client-a",
		pendingICE:   []webrtc.ICECandidateInit{{Candidate: "candidate"}},
		connected:    true,
		opusChannels: 2,
	}

	s.resetPeer(peer)

	if peer.connected {
		t.Fatal("expected peer to be disconnected")
	}
	if peer.pendingICE != nil {
		t.Fatalf("expected pending ICE to be cleared, got %#v", peer.pendingICE)
	}
	if peer.track != nil || peer.peer != nil {
		t.Fatalf("expected peer connection and track to be cleared")
	}
}
