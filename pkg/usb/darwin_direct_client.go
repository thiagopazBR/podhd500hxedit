//go:build darwin

package usb

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// warnf always reports transport problems (stderr), unlike debugf: a silent
// failure here means Mac->POD playback quietly stops.
func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[USB %s] "+format+"\n", append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}

// Client is the transport-independent API used by the tools: send a complete
// SPSI message, receive complete SPSI messages.
type Client interface {
	SendSPSI(spsi []byte) error
	RecvChan() <-chan *SPSIMessage
	Close() error
}

var (
	_ Client = (*DriverClient)(nil)
	_ Client = (*DirectClient)(nil)
)

// DirectClient talks to the POD HD500 control interface (USB interface #1)
// from user space, bypassing the Line 6 kext's kl6drvmsg layer.
//
// Requirements found on real hardware (2026-11-03):
//   - With the kext's bulkpipecomm released, USB playback (Mac audio out of
//     the POD) only works while the host keeps this interface open and keeps
//     re-applying the monitor level (periodically and after preset changes).
//   - Bulk IN must be drained continuously: if the reader stalls, the pedal
//     stops accepting bulk OUT and writes block. So the reader never writes;
//     it only signals the monitor goroutine, and every write has a timeout.
type DirectClient struct {
	ci       *ControlInterface
	recvCh   chan *SPSIMessage
	monitor  float32
	sendMu   sync.Mutex    // serializes whole SPSI/monitor writes; guards lastOut
	lastOut  time.Time     // end of the previous bulk OUT transfer
	kick     chan struct{} // reader -> monitor goroutine: re-apply level now
	stopCh   chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// DirectOptions configures OpenDirectClient.
type DirectOptions struct {
	// MonitorLevel is the USB playback level 0.0..1.0. Negative disables
	// all monitor handling (USB playback from the Mac will then be silent).
	MonitorLevel float32
	// Keepalive re-sends the monitor level at this interval (0 = only on
	// open and on preset/setlist changes).
	Keepalive time.Duration
}

// DefaultDirectOptions keeps Mac playback alive the same way it was verified
// on hardware (level 1.0, resent every 2s and after preset changes).
var DefaultDirectOptions = DirectOptions{MonitorLevel: 1.0, Keepalive: 2 * time.Second}

// OpenDirectClient opens interface #1 directly. The Line 6 bulkpipecomm
// driver must have been released first ('sudo l6-rebind release').
func OpenDirectClient(opts DirectOptions) (*DirectClient, error) {
	ci, err := OpenControlInterface()
	if err != nil {
		return nil, err
	}
	c := &DirectClient{
		ci:      ci,
		recvCh:  make(chan *SPSIMessage, 256),
		monitor: opts.MonitorLevel,
		kick:    make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
	}
	c.wg.Add(1)
	go c.readLoop() // start draining IN before the first write
	if c.monitor >= 0 {
		if err := c.applyMonitor(); err != nil {
			c.Close()
			return nil, fmt.Errorf("monitor level: %w", err)
		}
		c.wg.Add(1)
		go c.monitorLoop(opts.Keepalive)
	}
	return c, nil
}

// MinPacketGap is the minimum time between the end of one bulk OUT transfer
// and the start of the next. Measured on hardware (2026-11-03, cmd/abort-probe,
// 20 000 writes each): back-to-back writes were completed with
// kIOReturnAborted ~0.3% of the time, 200–500 µs gaps made it worse (up to
// 3.3%), and a 1 ms gap gave zero aborts, with or without the audio engine.
const MinPacketGap = time.Millisecond

func (c *DirectClient) writeRaw(b []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.pacedWrite(b)
}

// pacedWrite writes one packet, keeping MinPacketGap after the previous one.
// Caller holds sendMu.
func (c *DirectClient) pacedWrite(b []byte) error {
	if wait := MinPacketGap - time.Since(c.lastOut); wait > 0 {
		time.Sleep(wait)
	}
	err := c.ci.WriteRaw(b)
	c.lastOut = time.Now()
	return err
}

func (c *DirectClient) applyMonitor() error {
	return c.writeRaw(BuildMonitorLevelMsg(c.monitor))
}

// SendSPSI splits a complete SPSI message into 64-byte packets and writes
// them as consecutive transfers. The lock is held for the whole message so a
// keepalive can never land between its packets.
//
// The pedal assembles a message by the length in its SPSI header and does
// NOT restart on a new 0x01 packet (seen 2026-11-03: after a 4108-byte
// PRESET_SET stopped at packet 20 it swallowed every later packet). So a
// message must never be abandoned or restarted midway. Packets are paced by
// MinPacketGap, which removed the kIOReturnAborted (0xe00002eb) completions
// seen with back-to-back writes; as a safety net an aborted packet (a 64-byte
// transfer is one USB packet, so it was not acknowledged) is retried as-is.
func (c *DirectClient) SendSPSI(spsi []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	pkts := FrameSPSI(spsi)
	const attemptsPerPacket = 5
	for i, pkt := range pkts {
		var err error
		for a := 1; a <= attemptsPerPacket; a++ {
			if err = c.pacedWrite(pkt); err == nil || !strings.HasSuffix(err.Error(), "0xe00002eb") {
				break
			}
			warnf("pacote %d/%d abortado; repetindo o mesmo pacote (tentativa %d de %d)", i+1, len(pkts), a+1, attemptsPerPacket)
		}
		if err != nil {
			if i > 0 {
				warnf("mensagem interrompida no pacote %d/%d: o pedal pode ter ficado esperando o resto e ignorar os proximos comandos (desligue e ligue a POD)", i+1, len(pkts))
			}
			return fmt.Errorf("packet %d/%d: %w", i+1, len(pkts), err)
		}
	}
	return nil
}

// RecvChan returns complete SPSI messages received from the pedal.
func (c *DirectClient) RecvChan() <-chan *SPSIMessage { return c.recvCh }

func (c *DirectClient) stopping() bool {
	select {
	case <-c.stopCh:
		return true
	default:
		return false
	}
}

// readLoop drains bulk IN continuously. It must never block on anything but
// ReadRaw: no writes, no blocking channel sends.
func (c *DirectClient) readLoop() {
	defer c.wg.Done()
	var r Reassembler
	buf := make([]byte, 4096)
	failures, aborts := 0, 0
	for !c.stopping() {
		n, err := c.ci.ReadRaw(buf)
		if err != nil {
			if c.stopping() {
				return
			}
			if strings.HasSuffix(err.Error(), "0xe00002eb") {
				// kIOReturnAborted shows up intermittently under heavy IN
				// traffic without losing data (the pedal keeps it queued):
				// post the next read right away, but never spin if it repeats
				// (e.g. device gone).
				aborts++
				debugf("bulk IN read aborted (%d seguidas), retrying: %v", aborts, err)
				if aborts > 5 {
					if aborts == 6 {
						warnf("leitura bulk IN abortada repetidamente: %v", err)
					}
					time.Sleep(200 * time.Millisecond)
				}
				continue
			}
			failures++
			warnf("leitura bulk IN falhou (%d): %v", failures, err)
			if failures > 20 {
				warnf("leitura abandonada apos %d falhas seguidas; reinicie o programa", failures)
				return
			}
			if cerr := c.ci.ClearInStall(); cerr != nil {
				warnf("%v", cerr)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		failures, aborts = 0, 0
		if Debug {
			debugf("bulk IN %d bytes: %x", n, buf[:n])
		}
		for _, raw := range r.Feed(buf[:n]) {
			msg, err := ParseSPSI(raw)
			if err != nil {
				debugf("ParseSPSI: %v (%x)", err, raw)
				continue
			}
			if msg.Cmd == CmdPresetSelected || msg.Cmd == CmdSetSelected || msg.Cmd == CmdPresetLoad {
				// The pedal drops USB playback when it loads a preset.
				select {
				case c.kick <- struct{}{}:
				default: // a re-apply is already pending
				}
			}
			c.deliver(msg)
		}
	}
}

// deliver hands a message to the consumer without ever blocking the reader:
// if the consumer is behind, the oldest queued message is dropped.
func (c *DirectClient) deliver(msg *SPSIMessage) {
	for {
		select {
		case c.recvCh <- msg:
			return
		default:
		}
		select {
		case old := <-c.recvCh:
			warnf("consumidor atrasado: descartando %s", FormatCmd(old.Cmd))
		default:
		}
	}
}

// monitorLoop is the only background writer: it re-applies the USB playback
// level on demand (after preset changes) and every `every` (if > 0).
func (c *DirectClient) monitorLoop(every time.Duration) {
	defer c.wg.Done()
	var tick <-chan time.Time
	if every > 0 {
		t := time.NewTicker(every)
		defer t.Stop()
		tick = t.C
	}
	for {
		select {
		case <-c.stopCh:
			return
		case <-c.kick:
		case <-tick:
		}
		if err := c.applyMonitor(); err != nil && !c.stopping() {
			warnf("reaplicar volume de playback falhou: %v", err)
		}
	}
}

// Close stops the goroutines and releases the interface. USB playback from
// the Mac stops once the interface is closed.
func (c *DirectClient) Close() error {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		c.ci.Close() // aborts in-flight reads/writes, waits for them, releases
		c.wg.Wait()
	})
	return nil
}
