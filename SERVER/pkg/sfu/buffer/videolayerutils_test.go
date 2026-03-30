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

package buffer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"__GITHUB_HUBLIVE__protocol/codecs/mime"
	"__GITHUB_HUBLIVE__protocol/hublive"
)

func TestRidConversion(t *testing.T) {
	type RidAndLayer struct {
		rid   string
		layer int32
	}
	tests := []struct {
		name       string
		trackInfo  *hublive.TrackInfo
		mimeType   mime.MimeType
		ridToLayer map[string]RidAndLayer
	}{
		{
			"no track info",
			nil,
			mime.MimeTypeVP8,
			map[string]RidAndLayer{
				"":                 {rid: quarterResolutionQ, layer: 0},
				quarterResolutionQ: {rid: quarterResolutionQ, layer: 0},
				halfResolutionH:    {rid: halfResolutionH, layer: 1},
				fullResolutionF:    {rid: fullResolutionF, layer: 2},
			},
		},
		{
			"no layers",
			&hublive.TrackInfo{},
			mime.MimeTypeVP8,
			map[string]RidAndLayer{
				"":                 {rid: quarterResolutionQ, layer: 0},
				quarterResolutionQ: {rid: quarterResolutionQ, layer: 0},
				halfResolutionH:    {rid: halfResolutionH, layer: 1},
				fullResolutionF:    {rid: fullResolutionF, layer: 2},
			},
		},
		{
			"single layer, low",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]RidAndLayer{
				"":                 {rid: quarterResolutionQ, layer: 0},
				quarterResolutionQ: {rid: quarterResolutionQ, layer: 0},
				halfResolutionH:    {rid: quarterResolutionQ, layer: 0},
				fullResolutionF:    {rid: quarterResolutionQ, layer: 0},
			},
		},
		{
			"single layer, medium",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_MEDIUM},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]RidAndLayer{
				"":                 {rid: quarterResolutionQ, layer: 0},
				quarterResolutionQ: {rid: quarterResolutionQ, layer: 0},
				halfResolutionH:    {rid: quarterResolutionQ, layer: 0},
				fullResolutionF:    {rid: quarterResolutionQ, layer: 0},
			},
		},
		{
			"single layer, high",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_MEDIUM},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]RidAndLayer{
				"":                 {rid: quarterResolutionQ, layer: 0},
				quarterResolutionQ: {rid: quarterResolutionQ, layer: 0},
				halfResolutionH:    {rid: quarterResolutionQ, layer: 0},
				fullResolutionF:    {rid: quarterResolutionQ, layer: 0},
			},
		},
		{
			"two layers, low and medium",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
							{Quality: hublive.VideoQuality_MEDIUM},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]RidAndLayer{
				"":                 {rid: quarterResolutionQ, layer: 0},
				quarterResolutionQ: {rid: quarterResolutionQ, layer: 0},
				halfResolutionH:    {rid: halfResolutionH, layer: 1},
				fullResolutionF:    {rid: halfResolutionH, layer: 1},
			},
		},
		{
			"two layers, low and high",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]RidAndLayer{
				"":                 {rid: quarterResolutionQ, layer: 0},
				quarterResolutionQ: {rid: quarterResolutionQ, layer: 0},
				halfResolutionH:    {rid: halfResolutionH, layer: 1},
				fullResolutionF:    {rid: halfResolutionH, layer: 1},
			},
		},
		{
			"two layers, medium and high",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_MEDIUM},
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]RidAndLayer{
				"":                 {rid: quarterResolutionQ, layer: 0},
				quarterResolutionQ: {rid: quarterResolutionQ, layer: 0},
				halfResolutionH:    {rid: halfResolutionH, layer: 1},
				fullResolutionF:    {rid: halfResolutionH, layer: 1},
			},
		},
		{
			"three layers",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
							{Quality: hublive.VideoQuality_MEDIUM},
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]RidAndLayer{
				"":                 {rid: quarterResolutionQ, layer: 0},
				quarterResolutionQ: {rid: quarterResolutionQ, layer: 0},
				halfResolutionH:    {rid: halfResolutionH, layer: 1},
				fullResolutionF:    {rid: fullResolutionF, layer: 2},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for testRid, expectedResult := range test.ridToLayer {
				actualLayer := RidToSpatialLayer(test.mimeType, testRid, test.trackInfo, DefaultVideoLayersRid)
				require.Equal(t, expectedResult.layer, actualLayer)

				actualRid := SpatialLayerToRid(test.mimeType, actualLayer, test.trackInfo, DefaultVideoLayersRid)
				require.Equal(t, expectedResult.rid, actualRid)
			}
		})
	}
}

func TestQualityConversion(t *testing.T) {
	type QualityAndLayer struct {
		quality hublive.VideoQuality
		layer   int32
	}
	tests := []struct {
		name           string
		trackInfo      *hublive.TrackInfo
		mimeType       mime.MimeType
		qualityToLayer map[hublive.VideoQuality]QualityAndLayer
	}{
		{
			"no track info",
			nil,
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]QualityAndLayer{
				hublive.VideoQuality_LOW:    {quality: hublive.VideoQuality_LOW, layer: 0},
				hublive.VideoQuality_MEDIUM: {quality: hublive.VideoQuality_MEDIUM, layer: 1},
				hublive.VideoQuality_HIGH:   {quality: hublive.VideoQuality_HIGH, layer: 2},
			},
		},
		{
			"no layers",
			&hublive.TrackInfo{},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]QualityAndLayer{
				hublive.VideoQuality_LOW:    {quality: hublive.VideoQuality_LOW, layer: 0},
				hublive.VideoQuality_MEDIUM: {quality: hublive.VideoQuality_MEDIUM, layer: 1},
				hublive.VideoQuality_HIGH:   {quality: hublive.VideoQuality_HIGH, layer: 2},
			},
		},
		{
			"single layer, low",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]QualityAndLayer{
				hublive.VideoQuality_LOW:    {quality: hublive.VideoQuality_LOW, layer: 0},
				hublive.VideoQuality_MEDIUM: {quality: hublive.VideoQuality_LOW, layer: 0},
				hublive.VideoQuality_HIGH:   {quality: hublive.VideoQuality_LOW, layer: 0},
			},
		},
		{
			"single layer, medium",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_MEDIUM},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]QualityAndLayer{
				hublive.VideoQuality_LOW:    {quality: hublive.VideoQuality_MEDIUM, layer: 0},
				hublive.VideoQuality_MEDIUM: {quality: hublive.VideoQuality_MEDIUM, layer: 0},
				hublive.VideoQuality_HIGH:   {quality: hublive.VideoQuality_MEDIUM, layer: 0},
			},
		},
		{
			"single layer, high",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]QualityAndLayer{
				hublive.VideoQuality_LOW:    {quality: hublive.VideoQuality_HIGH, layer: 0},
				hublive.VideoQuality_MEDIUM: {quality: hublive.VideoQuality_HIGH, layer: 0},
				hublive.VideoQuality_HIGH:   {quality: hublive.VideoQuality_HIGH, layer: 0},
			},
		},
		{
			"two layers, low and medium",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
							{Quality: hublive.VideoQuality_MEDIUM},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]QualityAndLayer{
				hublive.VideoQuality_LOW:    {quality: hublive.VideoQuality_LOW, layer: 0},
				hublive.VideoQuality_MEDIUM: {quality: hublive.VideoQuality_MEDIUM, layer: 1},
				hublive.VideoQuality_HIGH:   {quality: hublive.VideoQuality_MEDIUM, layer: 1},
			},
		},
		{
			"two layers, low and high",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]QualityAndLayer{
				hublive.VideoQuality_LOW:    {quality: hublive.VideoQuality_LOW, layer: 0},
				hublive.VideoQuality_MEDIUM: {quality: hublive.VideoQuality_HIGH, layer: 1},
				hublive.VideoQuality_HIGH:   {quality: hublive.VideoQuality_HIGH, layer: 1},
			},
		},
		{
			"two layers, medium and high",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_MEDIUM},
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]QualityAndLayer{
				hublive.VideoQuality_LOW:    {quality: hublive.VideoQuality_MEDIUM, layer: 0},
				hublive.VideoQuality_MEDIUM: {quality: hublive.VideoQuality_MEDIUM, layer: 0},
				hublive.VideoQuality_HIGH:   {quality: hublive.VideoQuality_HIGH, layer: 1},
			},
		},
		{
			"three layers",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
							{Quality: hublive.VideoQuality_MEDIUM},
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]QualityAndLayer{
				hublive.VideoQuality_LOW:    {quality: hublive.VideoQuality_LOW, layer: 0},
				hublive.VideoQuality_MEDIUM: {quality: hublive.VideoQuality_MEDIUM, layer: 1},
				hublive.VideoQuality_HIGH:   {quality: hublive.VideoQuality_HIGH, layer: 2},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for testQuality, expectedResult := range test.qualityToLayer {
				actualLayer := VideoQualityToSpatialLayer(test.mimeType, testQuality, test.trackInfo)
				require.Equal(t, expectedResult.layer, actualLayer)

				actualQuality := SpatialLayerToVideoQuality(test.mimeType, actualLayer, test.trackInfo)
				require.Equal(t, expectedResult.quality, actualQuality)
			}
		})
	}
}

func TestVideoQualityToRidConversion(t *testing.T) {
	tests := []struct {
		name         string
		trackInfo    *hublive.TrackInfo
		mimeTye      mime.MimeType
		qualityToRid map[hublive.VideoQuality]string
	}{
		{
			"no track info",
			nil,
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]string{
				hublive.VideoQuality_LOW:    quarterResolutionQ,
				hublive.VideoQuality_MEDIUM: halfResolutionH,
				hublive.VideoQuality_HIGH:   fullResolutionF,
			},
		},
		{
			"no layers",
			&hublive.TrackInfo{},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]string{
				hublive.VideoQuality_LOW:    quarterResolutionQ,
				hublive.VideoQuality_MEDIUM: halfResolutionH,
				hublive.VideoQuality_HIGH:   fullResolutionF,
			},
		},
		{
			"single layer, low",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]string{
				hublive.VideoQuality_LOW:    quarterResolutionQ,
				hublive.VideoQuality_MEDIUM: quarterResolutionQ,
				hublive.VideoQuality_HIGH:   quarterResolutionQ,
			},
		},
		{
			"single layer, medium",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_MEDIUM},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]string{
				hublive.VideoQuality_LOW:    quarterResolutionQ,
				hublive.VideoQuality_MEDIUM: quarterResolutionQ,
				hublive.VideoQuality_HIGH:   quarterResolutionQ,
			},
		},
		{
			"single layer, high",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]string{
				hublive.VideoQuality_LOW:    quarterResolutionQ,
				hublive.VideoQuality_MEDIUM: quarterResolutionQ,
				hublive.VideoQuality_HIGH:   quarterResolutionQ,
			},
		},
		{
			"two layers, low and medium",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
							{Quality: hublive.VideoQuality_MEDIUM},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]string{
				hublive.VideoQuality_LOW:    quarterResolutionQ,
				hublive.VideoQuality_MEDIUM: halfResolutionH,
				hublive.VideoQuality_HIGH:   halfResolutionH,
			},
		},
		{
			"two layers, low and high",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]string{
				hublive.VideoQuality_LOW:    quarterResolutionQ,
				hublive.VideoQuality_MEDIUM: halfResolutionH,
				hublive.VideoQuality_HIGH:   halfResolutionH,
			},
		},
		{
			"two layers, medium and high",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_MEDIUM},
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]string{
				hublive.VideoQuality_LOW:    quarterResolutionQ,
				hublive.VideoQuality_MEDIUM: quarterResolutionQ,
				hublive.VideoQuality_HIGH:   halfResolutionH,
			},
		},
		{
			"three layers",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW},
							{Quality: hublive.VideoQuality_MEDIUM},
							{Quality: hublive.VideoQuality_HIGH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]string{
				hublive.VideoQuality_LOW:    quarterResolutionQ,
				hublive.VideoQuality_MEDIUM: halfResolutionH,
				hublive.VideoQuality_HIGH:   fullResolutionF,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for testQuality, expectedRid := range test.qualityToRid {
				actualRid := VideoQualityToRid(test.mimeTye, testQuality, test.trackInfo, DefaultVideoLayersRid)
				require.Equal(t, expectedRid, actualRid)
			}
		})
	}
}

func TestGetSpatialLayerForRid(t *testing.T) {
	tests := []struct {
		name              string
		trackInfo         *hublive.TrackInfo
		mimeType          mime.MimeType
		ridToSpatialLayer map[string]int32
	}{
		{
			"no track info",
			nil,
			mime.MimeTypeVP8,
			map[string]int32{
				quarterResolutionQ: InvalidLayerSpatial,
				halfResolutionH:    InvalidLayerSpatial,
				fullResolutionF:    InvalidLayerSpatial,
			},
		},
		{
			"no layers",
			&hublive.TrackInfo{},
			mime.MimeTypeVP8,
			map[string]int32{
				// SIMULCAST-CODEC-TODO
				// quarterResolutionQ: InvalidLayerSpatial,
				// halfResolutionH:    InvalidLayerSpatial,
				// fullResolutionF:    InvalidLayerSpatial,
				quarterResolutionQ: 0,
				halfResolutionH:    0,
				fullResolutionF:    0,
			},
		},
		{
			"no rid",
			&hublive.TrackInfo{},
			mime.MimeTypeVP8,
			map[string]int32{
				"": 0,
			},
		},
		{
			"single layer",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW, SpatialLayer: 0},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]int32{
				quarterResolutionQ: 0,
				halfResolutionH:    0,
				fullResolutionF:    0,
			},
		},
		{
			"layers",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW, SpatialLayer: 0, Rid: quarterResolutionQ},
							{Quality: hublive.VideoQuality_MEDIUM, SpatialLayer: 1, Rid: halfResolutionH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]int32{
				quarterResolutionQ: 0,
				halfResolutionH:    1,
				// SIMULCAST-CODEC-TODO
				// fullResolutionF:    InvalidLayerSpatial,
				fullResolutionF: 0,
			},
		},
		{
			"layers - no rid",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW, SpatialLayer: 0},
							{Quality: hublive.VideoQuality_MEDIUM, SpatialLayer: 1},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[string]int32{
				quarterResolutionQ: 0,
				halfResolutionH:    0,
				fullResolutionF:    0,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for testRid, expectedSpatialLayer := range test.ridToSpatialLayer {
				actualSpatialLayer := GetSpatialLayerForRid(test.mimeType, testRid, test.trackInfo)
				require.Equal(t, expectedSpatialLayer, actualSpatialLayer)
			}
		})
	}
}

func TestGetSpatialLayerForVideoQuality(t *testing.T) {
	tests := []struct {
		name                       string
		trackInfo                  *hublive.TrackInfo
		mimeType                   mime.MimeType
		videoQualityToSpatialLayer map[hublive.VideoQuality]int32
	}{
		{
			"no track info",
			nil,
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]int32{
				hublive.VideoQuality_LOW:    InvalidLayerSpatial,
				hublive.VideoQuality_MEDIUM: InvalidLayerSpatial,
				hublive.VideoQuality_HIGH:   InvalidLayerSpatial,
				hublive.VideoQuality_OFF:    InvalidLayerSpatial,
			},
		},
		{
			"no layers",
			&hublive.TrackInfo{},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]int32{
				hublive.VideoQuality_LOW:    0,
				hublive.VideoQuality_MEDIUM: 0,
				hublive.VideoQuality_HIGH:   0,
				hublive.VideoQuality_OFF:    InvalidLayerSpatial,
			},
		},
		{
			"not all layers",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW, SpatialLayer: 0, Rid: quarterResolutionQ},
							{Quality: hublive.VideoQuality_MEDIUM, SpatialLayer: 1, Rid: halfResolutionH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]int32{
				hublive.VideoQuality_LOW:    0,
				hublive.VideoQuality_MEDIUM: 1,
				hublive.VideoQuality_HIGH:   1,
				hublive.VideoQuality_OFF:    InvalidLayerSpatial,
			},
		},
		{
			"all layers",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW, SpatialLayer: 0, Rid: quarterResolutionQ},
							{Quality: hublive.VideoQuality_MEDIUM, SpatialLayer: 1, Rid: halfResolutionH},
							{Quality: hublive.VideoQuality_HIGH, SpatialLayer: 2, Rid: fullResolutionF},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[hublive.VideoQuality]int32{
				hublive.VideoQuality_LOW:    0,
				hublive.VideoQuality_MEDIUM: 1,
				hublive.VideoQuality_HIGH:   2,
				hublive.VideoQuality_OFF:    InvalidLayerSpatial,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for testVideoQuality, expectedSpatialLayer := range test.videoQualityToSpatialLayer {
				actualSpatialLayer := GetSpatialLayerForVideoQuality(test.mimeType, testVideoQuality, test.trackInfo)
				require.Equal(t, expectedSpatialLayer, actualSpatialLayer)
			}
		})
	}
}

func TestGetVideoQualityorSpatialLayer(t *testing.T) {
	tests := []struct {
		name                       string
		trackInfo                  *hublive.TrackInfo
		mimeType                   mime.MimeType
		spatialLayerToVideoQuality map[int32]hublive.VideoQuality
	}{
		{
			"no track info",
			nil,
			mime.MimeTypeVP8,
			map[int32]hublive.VideoQuality{
				InvalidLayerSpatial: hublive.VideoQuality_OFF,
				0:                   hublive.VideoQuality_OFF,
				1:                   hublive.VideoQuality_OFF,
				2:                   hublive.VideoQuality_OFF,
			},
		},
		{
			"no layers",
			&hublive.TrackInfo{},
			mime.MimeTypeVP8,
			map[int32]hublive.VideoQuality{
				InvalidLayerSpatial: hublive.VideoQuality_OFF,
				0:                   hublive.VideoQuality_OFF,
				1:                   hublive.VideoQuality_OFF,
				2:                   hublive.VideoQuality_OFF,
			},
		},
		{
			"layers",
			&hublive.TrackInfo{
				Codecs: []*hublive.SimulcastCodecInfo{
					{
						MimeType: mime.MimeTypeVP8.String(),
						Layers: []*hublive.VideoLayer{
							{Quality: hublive.VideoQuality_LOW, SpatialLayer: 0, Rid: quarterResolutionQ},
							{Quality: hublive.VideoQuality_MEDIUM, SpatialLayer: 1, Rid: halfResolutionH},
						},
					},
				},
			},
			mime.MimeTypeVP8,
			map[int32]hublive.VideoQuality{
				InvalidLayerSpatial: hublive.VideoQuality_OFF,
				0:                   hublive.VideoQuality_LOW,
				1:                   hublive.VideoQuality_MEDIUM,
				2:                   hublive.VideoQuality_OFF,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for testSpatialLayer, expectedVideoQuality := range test.spatialLayerToVideoQuality {
				actualVideoQuality := GetVideoQualityForSpatialLayer(test.mimeType, testSpatialLayer, test.trackInfo)
				require.Equal(t, expectedVideoQuality, actualVideoQuality)
			}
		})
	}
}

func TestNormalizeVideoLayersRid(t *testing.T) {
	tests := []struct {
		name       string
		rids       VideoLayersRid
		normalized VideoLayersRid
	}{
		{
			"empty",
			VideoLayersRid{},
			VideoLayersRid{},
		},
		{
			"unknown pattern",
			VideoLayersRid{"3", "2", "1"},
			VideoLayersRid{"3", "2", "1"},
		},
		{
			"qhf",
			videoLayersRidQHF,
			videoLayersRidQHF,
		},
		{
			"scrambled qhf",
			VideoLayersRid{"f", "h", "q"},
			videoLayersRidQHF,
		},
		{
			"partial qhf",
			VideoLayersRid{"h", "q"},
			VideoLayersRid{"q", "h", ""},
		},
		{
			"210",
			videoLayersRid210,
			videoLayersRid210,
		},
		{
			"scrambled 210",
			VideoLayersRid{"2", "0", "1"},
			videoLayersRid210,
		},
		{
			"partial 210",
			VideoLayersRid{"1", "2"},
			VideoLayersRid{"2", "1", ""},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalizedRids := NormalizeVideoLayersRid(test.rids)
			require.Equal(t, test.normalized, normalizedRids)
		})
	}
}
