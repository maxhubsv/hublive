// Copyright 2023 HubLive, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rtc

import (
	"errors"
	"io"
	"net"
	"strings"

	"github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
)

const (
	trackIdSeparator = "|"

	cMinIPTruncateLen = 8
)

func UnpackStreamID(packed string) (participantID hublive.ParticipantID, trackID hublive.TrackID) {
	parts := strings.Split(packed, trackIdSeparator)
	if len(parts) > 1 {
		return hublive.ParticipantID(parts[0]), hublive.TrackID(packed[len(parts[0])+1:])
	}
	return hublive.ParticipantID(packed), ""
}

func PackStreamID(participantID hublive.ParticipantID, trackID hublive.TrackID) string {
	return string(participantID) + trackIdSeparator + string(trackID)
}

func PackSyncStreamID(participantID hublive.ParticipantID, stream string) string {
	return string(participantID) + trackIdSeparator + stream
}

func StreamFromTrackSource(source hublive.TrackSource) string {
	// group camera/mic, screenshare/audio together
	switch source {
	case hublive.TrackSource_SCREEN_SHARE:
		return "screen"
	case hublive.TrackSource_SCREEN_SHARE_AUDIO:
		return "screen"
	case hublive.TrackSource_CAMERA:
		return "camera"
	case hublive.TrackSource_MICROPHONE:
		return "camera"
	}
	return "unknown"
}

func PackDataTrackLabel(participantID hublive.ParticipantID, trackID hublive.TrackID, label string) string {
	return string(participantID) + trackIdSeparator + string(trackID) + trackIdSeparator + label
}

func UnpackDataTrackLabel(packed string) (participantID hublive.ParticipantID, trackID hublive.TrackID, label string) {
	parts := strings.Split(packed, trackIdSeparator)
	if len(parts) != 3 {
		return "", hublive.TrackID(packed), ""
	}
	participantID = hublive.ParticipantID(parts[0])
	trackID = hublive.TrackID(parts[1])
	label = parts[2]
	return
}

func ToProtoTrackKind(kind webrtc.RTPCodecType) hublive.TrackType {
	switch kind {
	case webrtc.RTPCodecTypeVideo:
		return hublive.TrackType_VIDEO
	case webrtc.RTPCodecTypeAudio:
		return hublive.TrackType_AUDIO
	}
	panic("unsupported track direction")
}

func IsEOF(err error) bool {
	return err == io.ErrClosedPipe || err == io.EOF
}

func Recover(l logger.Logger) any {
	if l == nil {
		l = logger.GetLogger()
	}
	r := recover()
	if r != nil {
		var err error
		switch e := r.(type) {
		case string:
			err = errors.New(e)
		case error:
			err = e
		default:
			err = errors.New("unknown panic")
		}
		l.Errorw("recovered panic", err, "panic", r)
	}

	return r
}

// logger helpers
func LoggerWithParticipant(l logger.Logger, identity hublive.ParticipantIdentity, sid hublive.ParticipantID, isRemote bool) logger.Logger {
	values := make([]any, 0, 4)
	if identity != "" {
		values = append(values, "participant", identity)
	}
	if sid != "" {
		values = append(values, "participantID", sid)
	}
	values = append(values, "remote", isRemote)
	// enable sampling per participant
	return l.WithValues(values...)
}

func LoggerWithRoom(l logger.Logger, name hublive.RoomName, roomID hublive.RoomID) logger.Logger {
	values := make([]any, 0, 2)
	if name != "" {
		values = append(values, "room", name)
	}
	if roomID != "" {
		values = append(values, "roomID", roomID)
	}
	// also sample for the room
	return l.WithItemSampler().WithValues(values...)
}

func LoggerWithTrack(l logger.Logger, trackID hublive.TrackID, isRelayed bool) logger.Logger {
	// sampling not required because caller already passing in participant's logger
	if trackID != "" {
		return l.WithValues("trackID", trackID, "relayed", isRelayed)
	}
	return l
}

func LoggerWithPCTarget(l logger.Logger, target hublive.SignalTarget) logger.Logger {
	return l.WithValues("transport", target)
}

func LoggerWithCodecMime(l logger.Logger, mimeType mime.MimeType) logger.Logger {
	if mimeType != mime.MimeTypeUnknown {
		return l.WithValues("mime", mimeType.String())
	}
	return l
}

func MaybeTruncateIP(addr string) string {
	ipAddr := net.ParseIP(addr)
	if ipAddr == nil {
		return ""
	}

	if ipAddr.IsPrivate() || len(addr) <= cMinIPTruncateLen {
		return addr
	}

	return addr[:len(addr)-3] + "..."
}

func ChunkProtoBatch[T proto.Message](batch []T, target int) [][]T {
	var chunks [][]T
	var start, size int
	for i, m := range batch {
		s := proto.Size(m)
		if size+s > target {
			if start < i {
				chunks = append(chunks, batch[start:i])
			}
			start = i
			size = 0
		}
		size += s
	}
	if start < len(batch) {
		chunks = append(chunks, batch[start:])
	}
	return chunks
}

func IsRedEnabled(ti *hublive.TrackInfo) bool {
	if len(ti.Codecs) != 0 && ti.Codecs[0].MimeType != "" {
		return mime.IsMimeTypeStringRED(ti.Codecs[0].MimeType)
	}

	return !ti.GetDisableRed()
}
