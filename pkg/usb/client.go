package usb

import (
	"fmt"
	"os"
	"time"
)

// Debug enables raw logging (stderr) of every packet or callback delivered by
// the transport, before any filtering. Use it to tell "hardware sent nothing"
// apart from "we received it and dropped it".
var Debug bool

func debugf(format string, args ...any) {
	if Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

// warnf always reports transport problems (stderr), unlike debugf: a silent
// failure here means Mac->POD playback quietly stops.
func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[USB %s] "+format+"\n", append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}

// Client is the transport-independent API used by the tools: send a complete
// SPSI message, receive complete SPSI messages. It lives outside the
// platform-specific files so every OS backend (macOS today, Linux for the
// Raspberry Pi) can implement it.
type Client interface {
	SendSPSI(spsi []byte) error
	RecvChan() <-chan *SPSIMessage
	Close() error
}
