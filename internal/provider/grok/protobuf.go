package grok

import (
	"encoding/binary"
	"math"
	"time"
)

// Heuristic protobuf scanner ported from CodexBar's ProtobufScan in
// GrokWebBillingFetcher.swift. The grok.com billing response has no published
// schema, so we walk the wire format and pick out the fields we recognize:
// a fixed32 usage percent and a varint reset timestamp.

const (
	// minResetUnix/maxResetUnix bound the varint values that can be a unix
	// reset timestamp (future seconds).
	minResetUnix = 1_700_000_000
	maxResetUnix = 2_100_000_000
	// maxScanDepth mirrors the Swift depth cap: recursion happens while
	// depth < 4, so sub-messages nest at most 4 levels.
	maxScanDepth = 4
)

type fixed32Field struct {
	path  []uint64
	value float32
	order int
}

type varintField struct {
	path  []uint64
	value uint64
}

type protobufScan struct {
	fixed32Fields []fixed32Field
	varintFields  []varintField
}

func (s *protobufScan) merge(other protobufScan) {
	s.fixed32Fields = append(s.fixed32Fields, other.fixed32Fields...)
	s.varintFields = append(s.varintFields, other.varintFields...)
}

// billingSnapshot mirrors GrokWebBillingSnapshot.
type billingSnapshot struct {
	usedPercent *float64
	resetsAt    *time.Time
}

// parseGRPCWebResponse mirrors GrokWebBillingFetcher.parseGRPCWebResponse.
func parseGRPCWebResponse(data []byte, now time.Time) (billingSnapshot, error) {
	payloads := grpcWebDataFrames(data)
	if len(payloads) == 0 && looksLikeProtobufPayload(data) {
		payloads = [][]byte{data}
	}
	if len(payloads) == 0 {
		return billingSnapshot{}, &Error{Kind: kindEmptyResponse}
	}

	var scan protobufScan
	for _, p := range payloads {
		scan.merge(scanProtobuf(p, 0))
	}

	// usedPercent: a finite fixed32 in [0,100] whose field path ends in field 1;
	// smallest path depth wins, then earliest occurrence order.
	var parsedPercent *float64
	bestDepth, bestOrder := 1<<30, 1<<30
	for _, f := range scan.fixed32Fields {
		if f.path[len(f.path)-1] != 1 {
			continue
		}
		v := float64(f.value)
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
			continue
		}
		if parsedPercent == nil || len(f.path) < bestDepth || (len(f.path) == bestDepth && f.order < bestOrder) {
			parsedPercent = &v
			bestDepth, bestOrder = len(f.path), f.order
		}
	}

	// resetsAt: a future varint in the unix-second window; prefer path [1,5,1]
	// and take the earliest within that preference.
	var preferredReset, anyReset *time.Time
	for _, f := range scan.varintFields {
		if f.value < minResetUnix || f.value > maxResetUnix {
			continue
		}
		t := time.Unix(int64(f.value), 0)
		if !t.After(now) {
			continue
		}
		if anyReset == nil || t.Before(*anyReset) {
			anyReset = &t
		}
		if len(f.path) == 3 && f.path[0] == 1 && f.path[1] == 5 && f.path[2] == 1 {
			if preferredReset == nil || t.Before(*preferredReset) {
				preferredReset = &t
			}
		}
	}
	reset := preferredReset
	if reset == nil {
		reset = anyReset
	}

	// noUsageYet: no fixed32 fields anywhere, but a reset plus a usage-period
	// marker (path [1,6,...] or path [1,8,1] with value 1/2) means 0% used.
	hasUsagePeriod := false
	for _, f := range scan.varintFields {
		if len(f.path) >= 2 && f.path[0] == 1 && f.path[1] == 6 {
			hasUsagePeriod = true
			break
		}
		if len(f.path) == 3 && f.path[0] == 1 && f.path[1] == 8 && f.path[2] == 1 &&
			(f.value == 1 || f.value == 2) {
			hasUsagePeriod = true
			break
		}
	}
	noUsageYet := parsedPercent == nil && len(scan.fixed32Fields) == 0 && reset != nil && hasUsagePeriod
	if parsedPercent == nil && noUsageYet {
		zero := 0.0
		parsedPercent = &zero
	}
	if parsedPercent == nil {
		return billingSnapshot{}, &Error{Kind: kindParseFailed}
	}
	return billingSnapshot{usedPercent: parsedPercent, resetsAt: reset}, nil
}

// scanProtobuf walks the wire format and collects fixed32/varint fields with
// their message paths, mirroring ProtobufScan.scanProtobuf (including its
// byte-skip recovery on malformed fields and its depth cap).
func scanProtobuf(data []byte, depth int) protobufScan {
	scan, _ := scanProtobufAt(data, depth, nil, 0)
	return scan
}

func scanProtobufAt(data []byte, depth int, path []uint64, order int) (protobufScan, int) {
	var scan protobufScan
	nextOrder := order
	i := 0
	for i < len(data) {
		fieldStart := i
		key, ok := readVarint(data, &i)
		if !ok || key == 0 {
			i = fieldStart + 1 // skip one byte and resync
			continue
		}
		fieldNumber := key >> 3
		wireType := key & 0x07
		fieldPath := extendPath(path, fieldNumber)

		switch wireType {
		case 0:
			value, ok := readVarint(data, &i)
			if !ok {
				i = fieldStart + 1
				continue
			}
			scan.varintFields = append(scan.varintFields, varintField{path: fieldPath, value: value})
		case 1: // fixed64: skip
			if i+8 > len(data) {
				return scan, nextOrder
			}
			i += 8
		case 2: // length-delimited: recurse as a sub-message
			length, ok := readVarint(data, &i)
			if !ok || length > uint64(len(data)-i) {
				i = fieldStart + 1
				continue
			}
			start := i
			end := i + int(length)
			if depth < maxScanDepth {
				nested, nOrder := scanProtobufAt(data[start:end], depth+1, fieldPath, nextOrder)
				scan.merge(nested)
				nextOrder = nOrder
			}
			i = end
		case 5: // fixed32: candidate percent
			if i+4 > len(data) {
				return scan, nextOrder
			}
			scan.fixed32Fields = append(scan.fixed32Fields, fixed32Field{
				path:  fieldPath,
				value: math.Float32frombits(binary.LittleEndian.Uint32(data[i : i+4])),
				order: nextOrder,
			})
			nextOrder++
			i += 4
		default: // groups and unknown wire types: skip one byte
			i = fieldStart + 1
		}
	}
	return scan, nextOrder
}

func extendPath(path []uint64, field uint64) []uint64 {
	out := make([]uint64, len(path)+1)
	copy(out, path)
	out[len(path)] = field
	return out
}

func readVarint(data []byte, i *int) (uint64, bool) {
	var value uint64
	var shift uint64
	for *i < len(data) && shift < 64 {
		b := data[*i]
		*i++
		value |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return value, true
		}
		shift += 7
	}
	return 0, false
}
