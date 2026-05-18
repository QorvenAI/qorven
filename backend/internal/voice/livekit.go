// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/pion/webrtc/v4"
)

// LiveKit integration — production WebRTC rooms with AI agent as participant.
// The Soul joins a LiveKit room, listens to user audio, processes through the brain,
// and publishes audio response back.

type LiveKitConfig struct {
	Host      string // e.g. wss://your-livekit.cloud
	APIKey    string
	APISecret string
}

type LiveKitBridge struct {
	cfg      LiveKitConfig
	pipeline *VoicePipeline
}

func NewLiveKitBridge(cfg LiveKitConfig, pipeline *VoicePipeline) *LiveKitBridge {
	return &LiveKitBridge{cfg: cfg, pipeline: pipeline}
}

// CreateRoom creates a new LiveKit room for a voice session.
func (lk *LiveKitBridge) CreateRoom(ctx context.Context, roomName string) error {
	client := lksdk.NewRoomServiceClient(lk.cfg.Host, lk.cfg.APIKey, lk.cfg.APISecret)
	_, err := client.CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:            roomName,
		EmptyTimeout:    300,
		MaxParticipants: 2,
	})
	return err
}

// GenerateToken creates a JWT for a participant to join a room.
func (lk *LiveKitBridge) GenerateToken(roomName, participantName string, isAgent bool) (string, error) {
	at := auth.NewAccessToken(lk.cfg.APIKey, lk.cfg.APISecret)
	grant := &auth.VideoGrant{RoomJoin: true, Room: roomName}
	if isAgent {
		grant.RoomAdmin = true
	}
	at.AddGrant(grant).SetIdentity(participantName).SetValidFor(24 * time.Hour)
	return at.ToJWT()
}

// JoinAsAgent connects the Soul to a LiveKit room and starts processing audio.
func (lk *LiveKitBridge) JoinAsAgent(ctx context.Context, roomName, agentID, sessionID string) (*lksdk.Room, error) {
	identity := "soul-" + agentID[:min(len(agentID), 8)]

	// Use a pointer so the callback can reference the room after assignment
	var room *lksdk.Room
	var err error
	room, err = lksdk.ConnectToRoom(lk.cfg.Host, lksdk.ConnectInfo{
		APIKey:              lk.cfg.APIKey,
		APISecret:           lk.cfg.APISecret,
		RoomName:            roomName,
		ParticipantIdentity: identity,
	}, &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnTrackSubscribed: func(track *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
				slog.Info("livekit.track_subscribed", "participant", rp.Identity(), "track", pub.SID())
				go lk.processAudioTrack(ctx, track, room, sessionID)
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("join room: %w", err)
	}
	slog.Info("livekit.agent_joined", "room", roomName, "agent", identity)
	return room, nil
}

// processAudioTrack handles incoming audio from a user participant.
func (lk *LiveKitBridge) processAudioTrack(ctx context.Context, track *webrtc.TrackRemote, room *lksdk.Room, sessionID string) {
	slog.Info("livekit.processing_track", "track", track.ID())
	// LiveKit delivers RTP packets with Opus audio
	// Accumulate, decode, run through pipeline, publish response track
	// Full implementation needs Opus decoder + local audio track publish
}

// GetRoomInfo returns information about a LiveKit room.
func (lk *LiveKitBridge) GetRoomInfo(ctx context.Context, roomName string) (*livekit.Room, error) {
	client := lksdk.NewRoomServiceClient(lk.cfg.Host, lk.cfg.APIKey, lk.cfg.APISecret)
	rooms, err := client.ListRooms(ctx, &livekit.ListRoomsRequest{Names: []string{roomName}})
	if err != nil {
		return nil, err
	}
	if len(rooms.Rooms) == 0 {
		return nil, fmt.Errorf("room not found: %s", roomName)
	}
	return rooms.Rooms[0], nil
}

// DeleteRoom removes a LiveKit room.
func (lk *LiveKitBridge) DeleteRoom(ctx context.Context, roomName string) error {
	client := lksdk.NewRoomServiceClient(lk.cfg.Host, lk.cfg.APIKey, lk.cfg.APISecret)
	_, err := client.DeleteRoom(ctx, &livekit.DeleteRoomRequest{Room: roomName})
	return err
}
