// Copyright 2022 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/containers"
)

type CompoundKeyEncoder = func(*bytes.Buffer, ...any) []byte

// EncodeBlockKeyPrefix [48 Bit (BlockID) + 48 Bit (SegmentID)]
func EncodeBlockKeyPrefix(segmentId, blockId uint64) []byte {
	buf := make([]byte, 12)
	tempBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(tempBuf, segmentId)
	copy(buf[0:], tempBuf[2:])
	binary.BigEndian.PutUint64(tempBuf, blockId)
	copy(buf[6:], tempBuf[2:])
	return buf
}

func DecodeBlockKeyPrefix(rowid types.Rowid) (segmentId, blockId uint64) {
	tempBuf := make([]byte, 8)
	copy(tempBuf[2:], rowid[0:6])
	segmentId = binary.BigEndian.Uint64(tempBuf)
	copy(tempBuf[2:], rowid[6:12])
	blockId = binary.BigEndian.Uint64(tempBuf)
	return
}

func EncodePhyAddrKey(segmentId, blockId uint64, offset uint32) types.Rowid {
	prefix := EncodeBlockKeyPrefix(segmentId, blockId)
	return EncodePhyAddrKeyWithPrefix(prefix, offset)
}

func EncodePhyAddrKeyWithPrefix(prefix []byte, offset uint32) types.Rowid {
	var rowid types.Rowid
	copy(rowid[:12], prefix)
	binary.BigEndian.PutUint32(rowid[12:16], offset)
	return rowid
}

func DecodePhyAddrKeyFromValue(v any) (segmentId, blockId uint64, offset uint32) {
	rowid := v.(types.Rowid)
	return DecodePhyAddrKey(rowid)
}

func DecodePhyAddrKey(src types.Rowid) (segmentId, blockId uint64, offset uint32) {
	segmentId, blockId = DecodeBlockKeyPrefix(src)
	offset = binary.BigEndian.Uint32(src[12:])
	return
}

func EncodeTypedVals(w *bytes.Buffer, vals ...any) []byte {
	if w == nil {
		w = new(bytes.Buffer)
	} else {
		w.Reset()
	}
	for _, val := range vals {
		switch v := val.(type) {
		case []byte:
			_, _ = w.Write(v)
		case bool:
			_, _ = w.Write(types.EncodeFixed(v))
		case int8:
			_, _ = w.Write(types.EncodeFixed(v))
		case int16:
			_, _ = w.Write(types.EncodeFixed(v))
		case int32:
			_, _ = w.Write(types.EncodeFixed(v))
		case int64:
			_, _ = w.Write(types.EncodeFixed(v))
		case uint8:
			_, _ = w.Write(types.EncodeFixed(v))
		case uint16:
			_, _ = w.Write(types.EncodeFixed(v))
		case uint32:
			_, _ = w.Write(types.EncodeFixed(v))
		case uint64:
			_, _ = w.Write(types.EncodeFixed(v))
		case float32:
			_, _ = w.Write(types.EncodeFixed(v))
		case float64:
			_, _ = w.Write(types.EncodeFixed(v))
		case types.Date:
			_, _ = w.Write(types.EncodeFixed(v))
		case types.Datetime:
			_, _ = w.Write(types.EncodeFixed(v))
		case types.Timestamp:
			_, _ = w.Write(types.EncodeFixed(v))
		case types.Decimal64:
			_, _ = w.Write(types.EncodeFixed(v))
		case types.Decimal128:
			_, _ = w.Write(types.EncodeFixed(v))
		case types.TS:
			_, _ = w.Write(types.EncodeFixed(v))
		case types.Rowid:
			_, _ = w.Write(types.EncodeFixed(v))
		default:
			panic(fmt.Errorf("%T:%v not supported", v, v))
		}
	}
	return w.Bytes()
}

func EncodeTuple(w *bytes.Buffer, row uint32, cols ...containers.Vector) []byte {
	vs := make([]any, len(cols))
	for i := range vs {
		vs[i] = cols[i].Get(int(row))
	}
	return EncodeTypedVals(w, vs...)
}

func EncodeCompoundColumn(cols ...containers.Vector) (cc containers.Vector) {
	if len(cols) == 1 {
		cc = cols[0]
		return
	}
	cc = containers.MakeVector(types.CompoundKeyType, false)
	w := new(bytes.Buffer)
	for row := 0; row < cols[0].Length(); row++ {
		w.Reset()
		for i := range cols {
			_, _ = w.Write(types.EncodeValue(cols[i].Get(row), cols[i].GetType()))
		}
		cc.Append(w.Bytes())
	}
	return cc
}
