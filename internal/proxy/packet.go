package proxy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

const (
	SSLRequestCode    = 80877103 // 0x04D2162F
	CancelRequestCode = 80877102 // 0x04D2162E
	ProtocolVersion3  = 196608   // 0x00030000
)

var (
	ErrPacketTooShort = errors.New("packet too short")
	ErrInvalidPacket  = errors.New("invalid postgres startup packet")
)

// StartupPacket represents a decoded PostgreSQL client initiation request.
type StartupPacket struct {
	Length          int32
	ProtocolVersion int32
	IsSSLRequest    bool
	IsCancelRequest bool
	CancelPID       int32
	CancelKey       int32
	Parameters      map[string]string
	RawPayload      []byte
}

// ReadStartupPacket reads the initial packet sent by a PostgreSQL frontend.
func ReadStartupPacket(r io.Reader) (*StartupPacket, error) {
	var length int32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	if length < 8 || length > 10000 {
		return nil, ErrInvalidPacket
	}

	payload := make([]byte, length-4)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	protoCode := int32(binary.BigEndian.Uint32(payload[:4]))

	pkt := &StartupPacket{
		Length:          length,
		ProtocolVersion: protoCode,
		RawPayload:      payload,
		Parameters:      make(map[string]string),
	}

	if protoCode == SSLRequestCode {
		pkt.IsSSLRequest = true
		return pkt, nil
	}

	if protoCode == CancelRequestCode {
		pkt.IsCancelRequest = true
		if len(payload) >= 12 {
			pkt.CancelPID = int32(binary.BigEndian.Uint32(payload[4:8]))
			pkt.CancelKey = int32(binary.BigEndian.Uint32(payload[8:12]))
		}
		return pkt, nil
	}

	// Normal StartupMessage (Protocol 3.0)
	// Remaining bytes are null-terminated key-value pairs
	idx := 4
	for idx < len(payload) {
		if payload[idx] == 0 {
			break
		}

		// Read key
		nullKey := bytes.IndexByte(payload[idx:], 0)
		if nullKey == -1 {
			break
		}
		key := string(payload[idx : idx+nullKey])
		idx += nullKey + 1

		if idx >= len(payload) {
			break
		}

		// Read value
		nullVal := bytes.IndexByte(payload[idx:], 0)
		if nullVal == -1 {
			break
		}
		val := string(payload[idx : idx+nullVal])
		idx += nullVal + 1

		pkt.Parameters[key] = val
	}

	return pkt, nil
}

// EncodeStartupMessage re-serializes the StartupPacket with updated parameters.
func EncodeStartupMessage(params map[string]string) []byte {
	var buf bytes.Buffer

	// Placeholder for 4-byte length
	_ = binary.Write(&buf, binary.BigEndian, int32(0))

	// Protocol version 3.0
	_ = binary.Write(&buf, binary.BigEndian, int32(ProtocolVersion3))

	// Write key-value pairs
	// Keep "user" and "database" first if present
	preferredOrder := []string{"user", "database", "application_name", "client_encoding"}
	written := make(map[string]bool)

	for _, k := range preferredOrder {
		if v, ok := params[k]; ok {
			buf.WriteString(k)
			buf.WriteByte(0)
			buf.WriteString(v)
			buf.WriteByte(0)
			written[k] = true
		}
	}

	for k, v := range params {
		if !written[k] {
			buf.WriteString(k)
			buf.WriteByte(0)
			buf.WriteString(v)
			buf.WriteByte(0)
		}
	}

	// Final terminating null byte
	buf.WriteByte(0)

	data := buf.Bytes()
	totalLen := int32(len(data))
	binary.BigEndian.PutUint32(data[0:4], uint32(totalLen))

	return data
}
