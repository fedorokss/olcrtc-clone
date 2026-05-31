package videochannel

import (
	"encoding/binary"
	"errors"
)

const (
	protocolMagic   uint32 = 0x4f565632
	protocolVersion byte   = 3
	frameTypeData   byte   = 1
	frameTypeAck    byte   = 2
	frameRoleAny    byte   = 0
	frameRoleServer byte   = 1
	frameRoleClient byte   = 2

	frameBindingOff   = 7
	frameSeqOff       = 11
	frameCRCOff       = 15
	frameAckFragOff   = 19
	frameAckLen       = 21
	frameTotalLenOff  = 21
	frameFragIdxOff   = 25
	frameFragTotalOff = 27
	frameDataHdrLen   = 29
)

var (
	ErrFrameTooShort       = errors.New("frame too short")
	ErrUnexpectedMagic     = errors.New("unexpected frame magic")
	ErrUnexpectedVersion   = errors.New("unexpected frame version")
	ErrAckTooShort         = errors.New("ack frame too short")
	ErrDataTooShort        = errors.New("data frame too short")
	ErrUnexpectedFrameType = errors.New("unexpected frame type")
)

type transportFrame struct {
	typ       byte
	role      byte
	binding   uint32
	seq       uint32
	crc       uint32
	totalLen  uint32
	fragIdx   uint16
	fragTotal uint16
	payload   []byte
}

func encodeDataFrameForBinding(
	role byte,
	binding uint32,
	seq, crc uint32,
	totalLen, fragIdx, fragTotal int,
	payload []byte,
) []byte {
	out := make([]byte, frameDataHdrLen+len(payload))
	h := out[:frameDataHdrLen:frameDataHdrLen]
	binary.BigEndian.PutUint32(h[0:4], protocolMagic)
	h[4] = protocolVersion
	h[5] = frameTypeData
	h[6] = role
	binary.BigEndian.PutUint32(h[frameBindingOff:frameSeqOff], binding)
	binary.BigEndian.PutUint32(h[frameSeqOff:frameCRCOff], seq)
	binary.BigEndian.PutUint32(h[frameCRCOff:frameAckFragOff], crc)
	binary.BigEndian.PutUint32(h[frameTotalLenOff:frameFragIdxOff], uint32(totalLen))   //nolint:gosec,lll
	binary.BigEndian.PutUint16(h[frameFragIdxOff:frameFragTotalOff], uint16(fragIdx))   //nolint:gosec,lll
	binary.BigEndian.PutUint16(h[frameFragTotalOff:frameDataHdrLen], uint16(fragTotal)) //nolint:gosec,lll
	copy(out[frameDataHdrLen:], payload)
	return out
}

func encodeAckFrame(seq, crc uint32, fragIdx uint16) []byte {
	return encodeAckFrameForBinding(frameRoleAny, 0, seq, crc, fragIdx)
}

func encodeAckFrameForBinding(role byte, binding, seq, crc uint32, fragIdx uint16) []byte {
	out := make([]byte, frameAckLen)
	binary.BigEndian.PutUint32(out[0:4], protocolMagic)
	out[4] = protocolVersion
	out[5] = frameTypeAck
	out[6] = role
	binary.BigEndian.PutUint32(out[frameBindingOff:frameSeqOff], binding)
	binary.BigEndian.PutUint32(out[frameSeqOff:frameCRCOff], seq)
	binary.BigEndian.PutUint32(out[frameCRCOff:frameAckFragOff], crc)
	binary.BigEndian.PutUint16(out[frameAckFragOff:frameAckLen], fragIdx)
	return out
}

func decodeTransportFrame(data []byte) (transportFrame, error) {
	if err := validateFrameHeader(data); err != nil {
		return transportFrame{}, err
	}
	typ := data[5]
	if len(data) < frameSeqOff {
		return transportFrame{}, shortFrameError(typ)
	}
	frame := transportFrame{
		typ:     typ,
		role:    data[6],
		binding: binary.BigEndian.Uint32(data[frameBindingOff:frameSeqOff]),
	}
	switch typ {
	case frameTypeAck:
		return decodeAckBody(frame, data)
	case frameTypeData:
		return decodeDataBody(frame, data)
	default:
		return transportFrame{}, ErrUnexpectedFrameType
	}
}

func validateFrameHeader(data []byte) error {
	if len(data) < 6 {
		return ErrFrameTooShort
	}
	if binary.BigEndian.Uint32(data[0:4]) != protocolMagic {
		return ErrUnexpectedMagic
	}
	if data[4] != protocolVersion {
		return ErrUnexpectedVersion
	}
	return nil
}

func shortFrameError(typ byte) error {
	switch typ {
	case frameTypeAck:
		return ErrAckTooShort
	case frameTypeData:
		return ErrDataTooShort
	default:
		return ErrUnexpectedFrameType
	}
}

func decodeAckBody(frame transportFrame, data []byte) (transportFrame, error) {
	if len(data) < frameAckLen {
		return transportFrame{}, ErrAckTooShort
	}
	b := data[:frameAckLen]
	frame.seq = binary.BigEndian.Uint32(b[frameSeqOff:frameCRCOff])
	frame.crc = binary.BigEndian.Uint32(b[frameCRCOff:frameAckFragOff])
	frame.fragIdx = binary.BigEndian.Uint16(b[frameAckFragOff:frameAckLen])
	return frame, nil
}

func decodeDataBody(frame transportFrame, data []byte) (transportFrame, error) {
	if len(data) < frameDataHdrLen {
		return transportFrame{}, ErrDataTooShort
	}
	h := data[:frameDataHdrLen]
	frame.seq = binary.BigEndian.Uint32(h[frameSeqOff:frameCRCOff])
	frame.crc = binary.BigEndian.Uint32(h[frameCRCOff:frameAckFragOff])
	frame.totalLen = binary.BigEndian.Uint32(h[frameTotalLenOff:frameFragIdxOff])
	frame.fragIdx = binary.BigEndian.Uint16(h[frameFragIdxOff:frameFragTotalOff])
	frame.fragTotal = binary.BigEndian.Uint16(h[frameFragTotalOff:frameDataHdrLen])
	if n := len(data) - frameDataHdrLen; n > 0 {
		frame.payload = make([]byte, n)
		copy(frame.payload, data[frameDataHdrLen:])
	}
	return frame, nil
}
