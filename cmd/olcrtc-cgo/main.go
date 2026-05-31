package main

import "C"

import (
	"github.com/openlibrecommunity/olcrtc/mobile"
)

const errorResult = C.longlong(-1)

//export Ping
func Ping(
	carrierName *C.char,
	transportName *C.char,
	roomID *C.char,
	clientID *C.char,
	keyHex *C.char,
	socksPort C.longlong,
	timeoutMillis C.longlong,
	pingURL *C.char,
	vp8FPS C.longlong,
	vp8BatchSize C.longlong,
) C.longlong {
	result, err := mobile.Ping(
		goString(carrierName),
		goString(transportName),
		goString(roomID),
		goString(clientID),
		goString(keyHex),
		goInt(socksPort),
		goInt(timeoutMillis),
		goString(pingURL),
		goInt(vp8FPS),
		goInt(vp8BatchSize),
	)
	if err != nil {
		return errorResult
	}
	return C.longlong(result)
}

//export Check
func Check(
	carrierName *C.char,
	transportName *C.char,
	roomID *C.char,
	clientID *C.char,
	keyHex *C.char,
	socksPort C.longlong,
	timeoutMillis C.longlong,
	vp8FPS C.longlong,
	vp8BatchSize C.longlong,
) C.longlong {
	result, err := mobile.Check(
		goString(carrierName),
		goString(transportName),
		goString(roomID),
		goString(clientID),
		goString(keyHex),
		goInt(socksPort),
		goInt(timeoutMillis),
		goInt(vp8FPS),
		goInt(vp8BatchSize),
	)
	if err != nil {
		return errorResult
	}
	return C.longlong(result)
}

func goString(value *C.char) string {
	if value == nil {
		return ""
	}
	return C.GoString(value)
}

func goInt(value C.longlong) int {
	const maxInt = int(^uint(0) >> 1)
	const minInt = -maxInt - 1
	if value > C.longlong(maxInt) {
		return maxInt
	}
	if value < C.longlong(minInt) {
		return minInt
	}
	return int(value)
}

func main() {}
