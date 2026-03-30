package client

import (
	"github.com/frostbyte73/core"
	"github.com/maxhubsv/hublive-server/pkg/rtc/datatrack"
	"__GITHUB_HUBLIVE__protocol/hublive"
	"__GITHUB_HUBLIVE__protocol/logger"
	"go.uber.org/atomic"
)

type DataTrackRemote struct {
	publisherIdentity  hublive.ParticipantIdentity
	publisherID        hublive.ParticipantID
	handle             uint16
	trackID            hublive.TrackID
	logger             logger.Logger
	numReceivedPackets atomic.Uint32

	closed core.Fuse
}

func NewDataTrackRemote(
	publisherIdentity hublive.ParticipantIdentity,
	publisherID hublive.ParticipantID,
	handle uint16,
	trackID hublive.TrackID,
	logger logger.Logger,
) *DataTrackRemote {
	logger.Infow(
		"creating data track remote",
		"publisherIdentity", publisherIdentity,
		"publisherID", publisherID,
		"handle", handle,
		"trackID", trackID,
	)
	return &DataTrackRemote{
		publisherIdentity: publisherIdentity,
		publisherID:       publisherID,
		handle:            handle,
		trackID:           trackID,
		logger:            logger,
	}
}

func (d *DataTrackRemote) Close() {
	d.logger.Infow(
		"closing data track remote",
		"publisherIdentity", d.publisherIdentity,
		"publisherID", d.publisherID,
		"handle", d.handle,
		"trackID", d.trackID,
	)
	d.closed.Break()
}

func (d *DataTrackRemote) Handle() uint16 {
	return d.handle
}

func (d *DataTrackRemote) ID() hublive.TrackID {
	return d.trackID
}

func (d *DataTrackRemote) PacketReceived(packet *datatrack.Packet) {
	if d.closed.IsBroken() {
		return
	}

	valid := true
	if len(packet.Payload) == 0 {
		valid = false
	}
	for i := range packet.Payload {
		if packet.Payload[i] != byte(255-i) {
			valid = false
			break
		}
	}
	if valid {
		d.numReceivedPackets.Inc()
	}
}

func (d *DataTrackRemote) NumReceivedPackets() uint32 {
	return d.numReceivedPackets.Load()
}
