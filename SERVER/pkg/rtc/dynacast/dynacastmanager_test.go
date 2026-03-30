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
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/maxhubsv/hublive-server/pkg/rtc/types"
	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"
)

type testDynacastManagerListener struct {
	onSubscribedMaxQualityChange func(subscribedQualties []*hublive.SubscribedCodec)
	onSubscribedAudioCodecChange func(subscribedCodecs []*hublive.SubscribedAudioCodec)
}

func (t *testDynacastManagerListener) OnDynacastSubscribedMaxQualityChange(
	subscribedQualities []*hublive.SubscribedCodec,
	_maxSubscribedQualities []types.SubscribedCodecQuality,
) {
	t.onSubscribedMaxQualityChange(subscribedQualities)
}

func (t *testDynacastManagerListener) OnDynacastSubscribedAudioCodecChange(
	codecs []*hublive.SubscribedAudioCodec,
) {
	t.onSubscribedAudioCodecChange(codecs)
}

func TestSubscribedMaxQuality(t *testing.T) {
	t.Run("subscribers muted", func(t *testing.T) {
		var lock sync.Mutex
		actualSubscribedQualities := make([]*hublive.SubscribedCodec, 0)

		dm := NewDynacastManagerVideo(DynacastManagerVideoParams{
			Listener: &testDynacastManagerListener{
				onSubscribedMaxQualityChange: func(subscribedQualities []*hublive.SubscribedCodec) {
					lock.Lock()
					actualSubscribedQualities = subscribedQualities
					lock.Unlock()
				},
			},
		})

		dm.NotifySubscriberMaxQuality("s1", mime.MimeTypeVP8, hublive.VideoQuality_HIGH)
		dm.NotifySubscriberMaxQuality("s2", mime.MimeTypeAV1, hublive.VideoQuality_HIGH)

		// mute all subscribers of vp8
		dm.NotifySubscriberMaxQuality("s1", mime.MimeTypeVP8, hublive.VideoQuality_OFF)

		expectedSubscribedQualities := []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeVP8.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: false},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: false},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: true},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)
	})

	t.Run("subscribers max quality", func(t *testing.T) {
		lock := sync.RWMutex{}
		actualSubscribedQualities := make([]*hublive.SubscribedCodec, 0)

		dm := NewDynacastManagerVideo(DynacastManagerVideoParams{
			Listener: &testDynacastManagerListener{
				onSubscribedMaxQualityChange: func(subscribedQualities []*hublive.SubscribedCodec) {
					lock.Lock()
					actualSubscribedQualities = subscribedQualities
					lock.Unlock()
				},
			},
		})

		dm.(*dynacastManagerVideo).maxSubscribedQuality = map[mime.MimeType]hublive.VideoQuality{
			mime.MimeTypeVP8: hublive.VideoQuality_LOW,
			mime.MimeTypeAV1: hublive.VideoQuality_LOW,
		}
		dm.NotifySubscriberMaxQuality("s1", mime.MimeTypeVP8, hublive.VideoQuality_HIGH)
		dm.NotifySubscriberMaxQuality("s2", mime.MimeTypeVP8, hublive.VideoQuality_MEDIUM)
		dm.NotifySubscriberMaxQuality("s3", mime.MimeTypeAV1, hublive.VideoQuality_MEDIUM)

		expectedSubscribedQualities := []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeVP8.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: true},
				},
			},
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)

		// "s1" dropping to MEDIUM should disable HIGH layer
		dm.NotifySubscriberMaxQuality("s1", mime.MimeTypeVP8, hublive.VideoQuality_MEDIUM)

		expectedSubscribedQualities = []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeVP8.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)

		// "s1" , "s2" , "s3" dropping to LOW should disable HIGH & MEDIUM
		dm.NotifySubscriberMaxQuality("s1", mime.MimeTypeVP8, hublive.VideoQuality_LOW)
		dm.NotifySubscriberMaxQuality("s2", mime.MimeTypeVP8, hublive.VideoQuality_LOW)
		dm.NotifySubscriberMaxQuality("s3", mime.MimeTypeAV1, hublive.VideoQuality_LOW)

		expectedSubscribedQualities = []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeVP8.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: false},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: false},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)

		// muting "s2" only should not disable all qualities of vp8, no change of expected qualities
		dm.NotifySubscriberMaxQuality("s2", mime.MimeTypeVP8, hublive.VideoQuality_OFF)

		time.Sleep(100 * time.Millisecond)
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)

		// muting "s1" and s3 also should disable all qualities
		dm.NotifySubscriberMaxQuality("s1", mime.MimeTypeVP8, hublive.VideoQuality_OFF)
		dm.NotifySubscriberMaxQuality("s3", mime.MimeTypeAV1, hublive.VideoQuality_OFF)

		expectedSubscribedQualities = []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeVP8.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: false},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: false},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: false},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: false},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)

		// unmuting "s1" should enable vp8 previously set max quality
		dm.NotifySubscriberMaxQuality("s1", mime.MimeTypeVP8, hublive.VideoQuality_LOW)

		expectedSubscribedQualities = []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeVP8.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: false},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: false},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: false},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)

		// a higher quality from a different node should trigger that quality
		dm.NotifySubscriberNodeMaxQuality("n1", []types.SubscribedCodecQuality{
			{CodecMime: mime.MimeTypeVP8, Quality: hublive.VideoQuality_HIGH},
			{CodecMime: mime.MimeTypeAV1, Quality: hublive.VideoQuality_MEDIUM},
		})

		expectedSubscribedQualities = []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeVP8.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: true},
				},
			},
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)
	})
}

func TestCodecRegression(t *testing.T) {
	t.Run("codec regression video", func(t *testing.T) {
		var lock sync.Mutex
		actualSubscribedQualities := make([]*hublive.SubscribedCodec, 0)

		dm := NewDynacastManagerVideo(DynacastManagerVideoParams{
			Listener: &testDynacastManagerListener{
				onSubscribedMaxQualityChange: func(subscribedQualities []*hublive.SubscribedCodec) {
					lock.Lock()
					actualSubscribedQualities = subscribedQualities
					lock.Unlock()
				},
			},
		})

		dm.NotifySubscriberMaxQuality("s1", mime.MimeTypeAV1, hublive.VideoQuality_HIGH)

		expectedSubscribedQualities := []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: true},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)

		dm.HandleCodecRegression(mime.MimeTypeAV1, mime.MimeTypeVP8)

		expectedSubscribedQualities = []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: false},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: false},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
			{
				Codec: mime.MimeTypeVP8.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: true},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)

		// av1 quality change should be forwarded to vp8
		// av1 quality change of node should be ignored
		dm.NotifySubscriberMaxQuality("s1", mime.MimeTypeAV1, hublive.VideoQuality_MEDIUM)
		dm.NotifySubscriberNodeMaxQuality("n1", []types.SubscribedCodecQuality{
			{CodecMime: mime.MimeTypeAV1, Quality: hublive.VideoQuality_HIGH},
		})
		expectedSubscribedQualities = []*hublive.SubscribedCodec{
			{
				Codec: mime.MimeTypeAV1.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: false},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: false},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
			{
				Codec: mime.MimeTypeVP8.String(),
				Qualities: []*hublive.SubscribedQuality{
					{Quality: hublive.VideoQuality_LOW, Enabled: true},
					{Quality: hublive.VideoQuality_MEDIUM, Enabled: true},
					{Quality: hublive.VideoQuality_HIGH, Enabled: false},
				},
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedCodecsAsString(expectedSubscribedQualities) == subscribedCodecsAsString(actualSubscribedQualities)
		}, 10*time.Second, 100*time.Millisecond)
	})

	t.Run("codec regression audio", func(t *testing.T) {
		var lock sync.Mutex
		actualSubscribedCodecs := make([]*hublive.SubscribedAudioCodec, 0)

		dm := NewDynacastManagerAudio(DynacastManagerAudioParams{
			Listener: &testDynacastManagerListener{
				onSubscribedAudioCodecChange: func(subscribedCodecs []*hublive.SubscribedAudioCodec) {
					lock.Lock()
					actualSubscribedCodecs = subscribedCodecs
					lock.Unlock()
				},
			},
		})

		dm.NotifySubscription("s1", mime.MimeTypeRED, true)

		expectedSubscribedCodecs := []*hublive.SubscribedAudioCodec{
			{
				Codec:   mime.MimeTypeRED.String(),
				Enabled: true,
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedAudioCodecsAsString(expectedSubscribedCodecs) == subscribedAudioCodecsAsString(actualSubscribedCodecs)
		}, 10*time.Second, 100*time.Millisecond)

		dm.HandleCodecRegression(mime.MimeTypeRED, mime.MimeTypeOpus)

		expectedSubscribedCodecs = []*hublive.SubscribedAudioCodec{
			{
				Codec:   mime.MimeTypeRED.String(),
				Enabled: false,
			},
			{
				Codec:   mime.MimeTypeOpus.String(),
				Enabled: true,
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedAudioCodecsAsString(expectedSubscribedCodecs) == subscribedAudioCodecsAsString(actualSubscribedCodecs)
		}, 10*time.Second, 100*time.Millisecond)

		// RED disable as subscriber or subscriber node should be ignored as it has been regressed
		dm.NotifySubscription("s1", mime.MimeTypeRED, false)
		dm.NotifySubscriptionNode("n1", []*hublive.SubscribedAudioCodec{
			{Codec: mime.MimeTypeRED.String(), Enabled: false},
		})
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedAudioCodecsAsString(expectedSubscribedCodecs) == subscribedAudioCodecsAsString(actualSubscribedCodecs)
		}, 10*time.Second, 100*time.Millisecond)

		// `s1` unsubscription should turn off `opus`
		dm.NotifySubscription("s1", mime.MimeTypeOpus, false)
		expectedSubscribedCodecs = []*hublive.SubscribedAudioCodec{
			{
				Codec:   mime.MimeTypeRED.String(),
				Enabled: false,
			},
			{
				Codec:   mime.MimeTypeOpus.String(),
				Enabled: false,
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedAudioCodecsAsString(expectedSubscribedCodecs) == subscribedAudioCodecsAsString(actualSubscribedCodecs)
		}, 10*time.Second, 100*time.Millisecond)

		// a node subscription should turn `opus` back on
		dm.NotifySubscriptionNode("n1", []*hublive.SubscribedAudioCodec{
			{
				Codec:   mime.MimeTypeOpus.String(),
				Enabled: true,
			},
		})
		expectedSubscribedCodecs = []*hublive.SubscribedAudioCodec{
			{
				Codec:   mime.MimeTypeRED.String(),
				Enabled: false,
			},
			{
				Codec:   mime.MimeTypeOpus.String(),
				Enabled: true,
			},
		}
		require.Eventually(t, func() bool {
			lock.Lock()
			defer lock.Unlock()

			return subscribedAudioCodecsAsString(expectedSubscribedCodecs) == subscribedAudioCodecsAsString(actualSubscribedCodecs)
		}, 10*time.Second, 100*time.Millisecond)

	})
}

func subscribedCodecsAsString(c1 []*hublive.SubscribedCodec) string {
	sort.Slice(c1, func(i, j int) bool { return c1[i].Codec < c1[j].Codec })
	var s1 string
	for _, c := range c1 {
		s1 += c.String()
	}
	return s1
}

func subscribedAudioCodecsAsString(c1 []*hublive.SubscribedAudioCodec) string {
	sort.Slice(c1, func(i, j int) bool { return c1[i].Codec < c1[j].Codec })
	var s1 string
	for _, c := range c1 {
		s1 += c.String()
	}
	return s1
}
