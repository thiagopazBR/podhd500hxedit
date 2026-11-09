//go:build darwin

// l6-bulk talks to the POD HD500 control interface (USB interface #1) directly
// from user space, bypassing the Line 6 kext's message layer. Run
// 'sudo ./bin/l6-rebind release' first so the kext lets go of the interface.
//
//	l6-bulk -monitor 1.0 -listen 10s   # restore USB playback volume, then dump bulk IN
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	"podhd500-edit/pkg/usb"
)

func main() {
	monitor := flag.Float64("monitor", -1, "Send the USB playback (monitor) level 0.0..1.0 first (-1 = don't send)")
	listen := flag.Duration("listen", 10*time.Second, "How long to dump raw bulk IN (0x81) transfers")
	statusQuery := flag.Int("status-query", -1, "Send SPSI StatusQuery (0x21) with this status ID (8=preset, 9=setlist) after opening (-1 = don't send)")
	keepalive := flag.Duration("keepalive", 0, "Resend the monitor level at this interval while listening (0 = only once; needs -monitor)")
	quiet := flag.Bool("quiet", false, "Don't hexdump received transfers; just count them")
	flag.Parse()

	ci, err := usb.OpenControlInterface()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERRO]", err)
		os.Exit(1)
	}
	defer ci.Close()
	fmt.Println("[OK] Interface #1 da POD HD500 aberta em user space.")
	for _, p := range ci.Pipes {
		fmt.Printf("     pipe %d: EP 0x%02X tipo=%d maxPacket=%d intervalo=%d\n", p.Ref, p.Address(), p.TransferType, p.MaxPacketSize, p.Interval)
	}

	if *monitor >= 0 {
		msg := usb.BuildMonitorLevelMsg(float32(*monitor))
		fmt.Printf("[TX] Monitor Playback Volume = %.2f: %x\n", *monitor, msg)
		if err := ci.WriteRaw(msg); err != nil {
			fmt.Fprintln(os.Stderr, "[ERRO]", err)
		}
	}

	done := make(chan struct{})
	if *statusQuery >= 0 {
		spsi := usb.BuildStatusQuery(uint32(*statusQuery))
		// Single-chunk framing (refs/podhd_linux.c, kext BuildOutputFromL6SPSIAndSend):
		// [chunk payload length, 0, 0x01 = first chunk, 0] + payload.
		frame := append([]byte{byte(len(spsi)), 0x00, 0x01, 0x00}, spsi...)
		fmt.Printf("[TX] StatusQuery(%d): %x\n", *statusQuery, frame)
		if err := ci.WriteRaw(frame); err != nil {
			fmt.Fprintln(os.Stderr, "[ERRO]", err)
		}
	}

	fmt.Printf("[RX] Escutando bulk IN 0x81 por %v (pise nos footswitches)...\n", *listen)
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := ci.ReadRaw(buf)
			if err != nil {
				fmt.Println("[RX] leitura encerrada:", err)
				return
			}
			if !*quiet {
				fmt.Printf("[RX %s] %d bytes\n%s", time.Now().Format("15:04:05.000"), n, hex.Dump(buf[:n]))
			}
		}
	}()

	start := time.Now()
	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()
	var kaCh <-chan time.Time
	if *keepalive > 0 && *monitor >= 0 {
		t := time.NewTicker(*keepalive)
		defer t.Stop()
		kaCh = t.C
	}
	end := time.After(*listen)
loop:
	for {
		select {
		case <-heartbeat.C:
			fmt.Printf(">>> INTERFACE ABERTA (%2.0fs de %v) — o som do Mac esta saindo pela POD?\n", time.Since(start).Seconds(), *listen)
		case <-kaCh:
			if err := ci.WriteRaw(usb.BuildMonitorLevelMsg(float32(*monitor))); err != nil {
				fmt.Fprintln(os.Stderr, "[ERRO keepalive]", err)
			}
		case <-end:
			break loop
		}
	}
	fmt.Printf(">>> FECHANDO A INTERFACE AGORA (%s) — repare se o som do Mac para em seguida.\n", time.Now().Format("15:04:05"))
	ci.Close()
	<-done
}
