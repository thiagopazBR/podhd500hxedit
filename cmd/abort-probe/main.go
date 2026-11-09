//go:build darwin

// abort-probe measures how often bulk transfers on the POD HD500 control
// interface complete with kIOReturnAborted (0xe00002eb), under controlled
// conditions, to find what causes it. Only harmless traffic is sent: the USB
// playback level message (always the same value) and status queries.
//
//	-mode write       N single-packet writes, NO reader running
//	-mode write+read  same writes while a reader drains bulk IN concurrently
//	-mode query       N status queries (write, then wait for the reply)
//
// Requires 'sudo ./bin/l6-rebind release' first.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"podhd500-edit/pkg/usb"
)

type event struct {
	at   time.Duration
	what string
}

func main() {
	mode := flag.String("mode", "write", "write | write+read | query")
	n := flag.Int("n", 2000, "Number of writes / queries")
	interval := flag.Duration("interval", 0, "Pause between operations (0 = back to back)")
	level := flag.Float64("level", 1.0, "USB playback level sent in write modes")
	readBuf := flag.Int("readbuf", 4096, "Size of the pending bulk IN read buffer")
	flag.Parse()

	ci, err := usb.OpenControlInterface()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERRO]", err)
		os.Exit(1)
	}

	start := time.Now()
	var mu sync.Mutex
	var events []event
	record := func(what string) {
		mu.Lock()
		events = append(events, event{time.Since(start), what})
		mu.Unlock()
	}

	var readOK, readAbort, readOther atomic.Int64
	var gotReply atomic.Int64
	stopReader := make(chan struct{})
	readerDone := make(chan struct{})
	reader := *mode == "write+read" || *mode == "query"
	if reader {
		go func() {
			defer close(readerDone)
			buf := make([]byte, *readBuf)
			var r usb.Reassembler
			for {
				nr, err := ci.ReadRaw(buf)
				select {
				case <-stopReader:
					return
				default:
				}
				switch {
				case err == nil:
					readOK.Add(1)
					for _, m := range r.Feed(buf[:nr]) {
						if len(m) >= 8 && m[7] == usb.CmdStatusResp {
							gotReply.Add(1)
						}
					}
				case strings.HasSuffix(err.Error(), "0xe00002eb"):
					readAbort.Add(1)
					record("READ abortado")
				default:
					readOther.Add(1)
					record("READ erro: " + err.Error())
					time.Sleep(50 * time.Millisecond)
				}
			}
		}()
	} else {
		close(readerDone)
	}

	var writeOK, writeAbort, writeOther int
	var replyTimeouts int
	var worst time.Duration
	msg := usb.BuildMonitorLevelMsg(float32(*level))
	query := usb.FrameSPSI(usb.BuildStatusQuery(usb.StatusIDCurrentSet))[0]

	fmt.Printf("modo=%s n=%d intervalo=%v leitor=%v readbuf=%d\n", *mode, *n, *interval, reader, *readBuf)
	for i := 0; i < *n; i++ {
		pkt := msg
		if *mode == "query" {
			pkt = query
		}
		before := gotReply.Load()
		t0 := time.Now()
		err := ci.WriteRawTimeout(pkt, 2*time.Second)
		if d := time.Since(t0); d > worst {
			worst = d
		}
		switch {
		case err == nil:
			writeOK++
		case strings.HasSuffix(err.Error(), "0xe00002eb"):
			writeAbort++
			record(fmt.Sprintf("WRITE abortado (op %d) apos %v", i, time.Since(t0).Round(time.Microsecond)))
		default:
			writeOther++
			record(fmt.Sprintf("WRITE erro (op %d): %v", i, err))
		}
		if *mode == "query" && err == nil {
			deadline := time.Now().Add(time.Second)
			for gotReply.Load() == before && time.Now().Before(deadline) {
				time.Sleep(200 * time.Microsecond)
			}
			if gotReply.Load() == before {
				replyTimeouts++
				record(fmt.Sprintf("sem resposta (op %d)", i))
			}
		}
		if *interval > 0 {
			time.Sleep(*interval)
		}
	}
	elapsed := time.Since(start)

	close(stopReader)
	ci.Close()
	<-readerDone

	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("duracao: %v  (%.0f ops/s)\n", elapsed.Round(time.Millisecond), float64(*n)/elapsed.Seconds())
	fmt.Printf("WRITE: ok=%d abortado=%d outro_erro=%d  (pior latencia %v)\n", writeOK, writeAbort, writeOther, worst.Round(time.Microsecond))
	if reader {
		fmt.Printf("READ:  ok=%d abortado=%d outro_erro=%d\n", readOK.Load(), readAbort.Load(), readOther.Load())
	}
	if *mode == "query" {
		fmt.Printf("respostas STATUS_RESP=%d  sem_resposta=%d\n", gotReply.Load(), replyTimeouts)
	}
	fmt.Printf("AbortPipe/ClearPipeStall chamados pelo nosso codigo: %d\n", usb.OwnAbortCalls.Load())

	sort.Slice(events, func(i, j int) bool { return events[i].at < events[j].at })
	if len(events) > 0 {
		fmt.Println("eventos (tempo desde o inicio | intervalo desde o anterior):")
		var prev time.Duration
		for i, e := range events {
			if i == 60 {
				fmt.Printf("  ... mais %d eventos\n", len(events)-60)
				break
			}
			fmt.Printf("  %9.1f ms | +%8.1f ms | %s\n", ms(e.at), ms(e.at-prev), e.what)
			prev = e.at
		}
	}
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
