// Copyright 2024 StreamNative, Inc.
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

package codec

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/streamnative/oxia/server/util/crc"
)

func TestV2_GetHeaderSize(t *testing.T) {
	assert.EqualValues(t, v2.GetHeaderSize(), 12)
}

func TestV2_Codec(t *testing.T) {
	buf := make([]byte, 100)
	payload := []byte{1}
	recordSize, _ := v2.WriteRecord(buf, 0, 0, payload)
	assert.EqualValues(t, recordSize, 13)
	getRecordSize, err := v2.GetRecordSize(buf, 0)
	assert.NoError(t, err)
	assert.EqualValues(t, getRecordSize, recordSize)
	payloadSize, previousCrc, payloadCrc, err := v2.ReadHeaderWithValidation(buf, 0)
	assert.NoError(t, err)
	assert.EqualValues(t, previousCrc, 0)
	expectedPayloadCrc := crc.Checksum(0).Update(payload).Value()
	assert.EqualValues(t, expectedPayloadCrc, payloadCrc)
	assert.EqualValues(t, recordSize-(v2PayloadSizeLen+v2PreviousCrcLen+v2PayloadCrcLen), payloadSize)

	getPayload, err := v2.ReadRecordWithValidation(buf, 0)
	assert.NoError(t, err)
	assert.EqualValues(t, payload, getPayload)
}

func TestV2_Crc(t *testing.T) {
	buf := make([]byte, 1000)

	var entriesToOffset []uint32
	offset := uint32(0)
	previousCrc := uint32(0)
	for i := 0; i < 10; i++ {
		recordSize, payloadCrc := v2.WriteRecord(buf, offset, previousCrc, []byte{byte(i)})
		entriesToOffset = append(entriesToOffset, offset)
		previousCrc = payloadCrc
		offset += recordSize
	}

	// crc validation
	expectedChecksum := uint32(0)
	for index := range len(entriesToOffset) {
		startFOffset := entriesToOffset[index]
		_, previousCrc, payloadCrc, err := v2.ReadHeaderWithValidation(buf, startFOffset)
		assert.NoError(t, err)
		assert.EqualValues(t, expectedChecksum, previousCrc)
		expectedPayloadChecksum := crc.Checksum(expectedChecksum).Update([]byte{byte(index)}).Value()
		assert.EqualValues(t, expectedPayloadChecksum, payloadCrc)
		expectedChecksum = expectedPayloadChecksum
	}

	for index := range len(entriesToOffset) {
		startFOffset := entriesToOffset[index]
		payload, err := v2.ReadRecordWithValidation(buf, startFOffset)
		assert.NoError(t, err)
		assert.EqualValues(t, []byte{byte(index)}, payload)
	}
}

func TestV2_CrcConsistency(t *testing.T) {
	buf1 := make([]byte, 1000)
	buf2 := make([]byte, 1000)
	offset := uint32(0)
	previousCrc := uint32(0)

	// load buf-1
	var entriesToOffset1 []uint32

	for i := 0; i < 10; i++ {
		recordSize, payloadCrc := v2.WriteRecord(buf1, offset, previousCrc, []byte{byte(i)})
		entriesToOffset1 = append(entriesToOffset1, offset)
		previousCrc = payloadCrc
		offset += recordSize
	}

	// reload variables
	offset = uint32(0)
	previousCrc = uint32(0)

	// load buf-2
	var entriesToOffset2 []uint32
	for i := 0; i < 10; i++ {
		recordSize, payloadCrc := v2.WriteRecord(buf2, offset, previousCrc, []byte{byte(i)})
		entriesToOffset2 = append(entriesToOffset2, offset)
		previousCrc = payloadCrc
		offset += recordSize
	}

	lastEntryOffset1 := entriesToOffset1[len(entriesToOffset1)-1]
	lastEntryOffset2 := entriesToOffset2[len(entriesToOffset2)-1]

	_, _, payloadCrc1, err := v2.ReadHeaderWithValidation(buf1, lastEntryOffset1)
	assert.NoError(t, err)
	_, _, payloadCrc2, err := v2.ReadHeaderWithValidation(buf2, lastEntryOffset2)
	assert.NoError(t, err)
	assert.EqualValues(t, payloadCrc1, payloadCrc2)
}

func TestV2_DeviatingCrc(t *testing.T) {
	buf1 := make([]byte, 1000)
	buf2 := make([]byte, 1000)
	deviatingIndex := 5
	offset := uint32(0)
	previousCrc := uint32(0)

	// load buf-1
	var entriesToOffset1 []uint32

	for i := 0; i < 10; i++ {
		recordSize, payloadCrc := v2.WriteRecord(buf1, offset, previousCrc, []byte{byte(i)})
		entriesToOffset1 = append(entriesToOffset1, offset)
		previousCrc = payloadCrc
		offset += recordSize
	}

	// reload variables
	offset = uint32(0)
	previousCrc = uint32(0)

	// load buf-2
	var entriesToOffset2 []uint32
	for i := 0; i < 10; i++ {
		payload := []byte{byte(i)}
		if i == deviatingIndex {
			payload = []byte{128}
		}
		recordSize, payloadCrc := v2.WriteRecord(buf2, offset, previousCrc, payload)
		entriesToOffset2 = append(entriesToOffset2, offset)
		previousCrc = payloadCrc
		offset += recordSize
	}

	lastEntryOffset1 := entriesToOffset1[len(entriesToOffset1)-1]
	lastEntryOffset2 := entriesToOffset2[len(entriesToOffset2)-1]

	_, _, payloadCrc1, err := v2.ReadHeaderWithValidation(buf1, lastEntryOffset1)
	assert.NoError(t, err)
	_, _, payloadCrc2, err := v2.ReadHeaderWithValidation(buf2, lastEntryOffset2)
	assert.NoError(t, err)
	assert.NotEqualValues(t, payloadCrc1, payloadCrc2)

	assert.EqualValues(t, len(entriesToOffset1), len(entriesToOffset2))
	var actualDeviatingIndex int
	// find the deviating index
	for index := range len(entriesToOffset1) {
		fOffset := entriesToOffset1[index]
		_, _, payloadCrc1, err := v2.ReadHeaderWithValidation(buf1, fOffset)
		assert.NoError(t, err)
		fOffset = entriesToOffset2[index]
		_, _, payloadCrc2, err := v2.ReadHeaderWithValidation(buf2, fOffset)
		assert.NoError(t, err)
		if payloadCrc1 == payloadCrc2 {
			continue
		}
		actualDeviatingIndex = index
		break
	}
	assert.EqualValues(t, deviatingIndex, actualDeviatingIndex)
}

func TestV2_BreakingPoint_Size(t *testing.T) {
	buf := make([]byte, 100)

	v2.WriteRecord(buf, 0, 0, []byte{1})
	binary.BigEndian.PutUint32(buf, 123123)

	_, _, _, err := v2.ReadHeaderWithValidation(buf, 0)
	assert.ErrorIs(t, err, ErrOffsetOutOfBounds)

	_, err = v2.ReadRecordWithValidation(buf, 0)
	assert.ErrorIs(t, err, ErrOffsetOutOfBounds)
}

func TestV2_BreakingPoint_PreviousCrc(t *testing.T) {
	buf := make([]byte, 100)

	v2.WriteRecord(buf, 0, 0, []byte{1})
	binary.BigEndian.PutUint32(buf[v2PayloadSizeLen:], 123123)

	_, _, _, err := v2.ReadHeaderWithValidation(buf, 0)
	assert.ErrorIs(t, err, ErrDataCorrupted)

	_, err = v2.ReadRecordWithValidation(buf, 0)
	assert.ErrorIs(t, err, ErrDataCorrupted)
}

func TestV2_BreakingPoint_PayloadCrc(t *testing.T) {
	buf := make([]byte, 100)

	v2.WriteRecord(buf, 0, 0, []byte{1})
	binary.BigEndian.PutUint32(buf[v2PayloadSizeLen+v2PreviousCrcLen:], 123123)

	_, _, _, err := v2.ReadHeaderWithValidation(buf, 0)
	assert.ErrorIs(t, err, ErrDataCorrupted)

	_, err = v2.ReadRecordWithValidation(buf, 0)
	assert.ErrorIs(t, err, ErrDataCorrupted)
}

func TestV2_BreakingPoint_Payload(t *testing.T) {
	buf := make([]byte, 100)

	v2.WriteRecord(buf, 0, 0, []byte{1})
	binary.BigEndian.PutUint32(buf[v2.HeaderSize:], 1231242)

	_, _, _, err := v2.ReadHeaderWithValidation(buf, 0)
	assert.ErrorIs(t, err, ErrDataCorrupted)

	_, err = v2.ReadRecordWithValidation(buf, 0)
	assert.ErrorIs(t, err, ErrDataCorrupted)
}
