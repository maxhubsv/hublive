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

package dynacast

import (
	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"

	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
)

type DynacastManagerListener interface {
	OnDynacastSubscribedMaxQualityChange(
		subscribedQualities []*hublive.SubscribedCodec,
		maxSubscribedQualities []types.SubscribedCodecQuality,
	)

	OnDynacastSubscribedAudioCodecChange(codecs []*hublive.SubscribedAudioCodec)
}

var _ DynacastManagerListener = (*DynacastManagerListenerNull)(nil)

type DynacastManagerListenerNull struct {
}

func (d *DynacastManagerListenerNull) OnDynacastSubscribedMaxQualityChange(
	subscribedQualities []*hublive.SubscribedCodec,
	maxSubscribedQualities []types.SubscribedCodecQuality,
) {
}
func (d *DynacastManagerListenerNull) OnDynacastSubscribedAudioCodecChange(
	codecs []*hublive.SubscribedAudioCodec,
) {
}

// -----------------------------------------

type DynacastManager interface {
	AddCodec(mime mime.MimeType)
	HandleCodecRegression(fromMime, toMime mime.MimeType)
	Restart()
	Close()
	ForceUpdate()
	ForceQuality(quality hublive.VideoQuality)
	ForceEnable(enabled bool)

	NotifySubscriberMaxQuality(
		subscriberID hublive.ParticipantID,
		mime mime.MimeType,
		quality hublive.VideoQuality,
	)
	NotifySubscription(
		subscriberID hublive.ParticipantID,
		mime mime.MimeType,
		enabled bool,
	)

	NotifySubscriberNodeMaxQuality(
		nodeID hublive.NodeID,
		qualities []types.SubscribedCodecQuality,
	)
	NotifySubscriptionNode(
		nodeID hublive.NodeID,
		codecs []*hublive.SubscribedAudioCodec,
	)
	ClearSubscriberNodes()
}

var _ DynacastManager = (*dynacastManagerNull)(nil)

type dynacastManagerNull struct {
}

func (d *dynacastManagerNull) AddCodec(mime mime.MimeType)                          {}
func (d *dynacastManagerNull) HandleCodecRegression(fromMime, toMime mime.MimeType) {}
func (d *dynacastManagerNull) Restart()                                             {}
func (d *dynacastManagerNull) Close()                                               {}
func (d *dynacastManagerNull) ForceUpdate()                                         {}
func (d *dynacastManagerNull) ForceQuality(quality hublive.VideoQuality)            {}
func (d *dynacastManagerNull) ForceEnable(enabled bool)                             {}
func (d *dynacastManagerNull) NotifySubscriberMaxQuality(
	subscriberID hublive.ParticipantID,
	mime mime.MimeType,
	quality hublive.VideoQuality,
) {
}
func (d *dynacastManagerNull) NotifySubscription(
	subscriberID hublive.ParticipantID,
	mime mime.MimeType,
	enabled bool,
) {
}
func (d *dynacastManagerNull) NotifySubscriberNodeMaxQuality(
	nodeID hublive.NodeID,
	qualities []types.SubscribedCodecQuality,
) {
}
func (d *dynacastManagerNull) NotifySubscriptionNode(
	nodeID hublive.NodeID,
	codecs []*hublive.SubscribedAudioCodec,
) {
}
func (d *dynacastManagerNull) ClearSubscriberNodes() {}

// ------------------------------------------------

type dynacastQualityListener interface {
	OnUpdateMaxQualityForMime(mimeType mime.MimeType, maxQuality hublive.VideoQuality)
	OnUpdateAudioCodecForMime(mimeType mime.MimeType, enabled bool)
}

var _ dynacastQualityListener = (*dynacastQualityListenerNull)(nil)

type dynacastQualityListenerNull struct {
}

func (d *dynacastQualityListenerNull) OnUpdateMaxQualityForMime(
	mimeType mime.MimeType,
	maxQuality hublive.VideoQuality,
) {
}

func (d *dynacastQualityListenerNull) OnUpdateAudioCodecForMime(
	mimeType mime.MimeType,
	enabled bool,
) {
}

// ------------------------------------------------

type dynacastQuality interface {
	Start()
	Restart()
	Stop()

	NotifySubscriberMaxQuality(subscriberID hublive.ParticipantID, quality hublive.VideoQuality)
	NotifySubscription(subscriberID hublive.ParticipantID, enabled bool)

	NotifySubscriberNodeMaxQuality(nodeID hublive.NodeID, quality hublive.VideoQuality)
	NotifySubscriptionNode(nodeID hublive.NodeID, enabled bool)
	ClearSubscriberNodes()

	Replace(
		maxSubscriberQuality map[hublive.ParticipantID]hublive.VideoQuality,
		maxSubscriberNodeQuality map[hublive.NodeID]hublive.VideoQuality,
	)

	Mime() mime.MimeType
	RegressTo(other dynacastQuality)
}

var _ dynacastQuality = (*dynacastQualityNull)(nil)

type dynacastQualityNull struct {
}

func (d *dynacastQualityNull) Start()   {}
func (d *dynacastQualityNull) Restart() {}
func (d *dynacastQualityNull) Stop()    {}
func (d *dynacastQualityNull) NotifySubscriberMaxQuality(subscriberID hublive.ParticipantID, quality hublive.VideoQuality) {
}
func (d *dynacastQualityNull) NotifySubscription(subscriberID hublive.ParticipantID, enabled bool) {}
func (d *dynacastQualityNull) NotifySubscriberNodeMaxQuality(nodeID hublive.NodeID, quality hublive.VideoQuality) {
}
func (d *dynacastQualityNull) NotifySubscriptionNode(nodeID hublive.NodeID, enabled bool) {}
func (d *dynacastQualityNull) ClearSubscriberNodes()                                      {}
func (d *dynacastQualityNull) Replace(
	maxSubscriberQuality map[hublive.ParticipantID]hublive.VideoQuality,
	maxSubscriberNodeQuality map[hublive.NodeID]hublive.VideoQuality,
) {
}
func (d *dynacastQualityNull) Mime() mime.MimeType             { return mime.MimeTypeUnknown }
func (d *dynacastQualityNull) RegressTo(other dynacastQuality) {}
