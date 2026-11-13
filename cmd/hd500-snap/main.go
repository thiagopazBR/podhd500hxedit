// hd500-snap is the live snapshot player for the POD HD500. The pedal has no
// snapshots: they are made in the editor (hd500-edit, "Snapshots" menu) and
// saved as JSON files; this program plays them. It applies snapshot 1 of every
// preset that gets loaded, and applies snapshot N when asked to (today over
// HTTP on this machine; the pedal's own footswitches over MIDI come next).
//
// It only changes block on/off and parameter values through the editor's own
// writers; it never saves or edits a snapshot and never writes to the pedal's
// memory. The HTTP trigger has no authentication: it listens on loopback only.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"podhd500-edit/pkg/editor"
	"podhd500-edit/pkg/midiin"
	"podhd500-edit/pkg/player"
	"podhd500-edit/pkg/usb"
)

func main() {
	usb.Debug = os.Getenv("HD500_USB_DEBUG") == "1"
	midiin.Debug = os.Getenv("HD500_MIDI_DEBUG") == "1"
	addr := flag.String("http", "127.0.0.1:8600", "HTTP trigger address (loopback only): POST /snapshot/{1..4}, GET /status; empty = off")
	dir := flag.String("dir", "", "folder of the snapshot files (default: the editor's own, "+editor.DefaultSnapshotDir()+")")
	midiMatch := flag.String("midi", "", "MIDI input to listen to (part of its name, e.g. \"EVO8\"): CC -midi-cc..+3 = snapshots 1..4")
	midiList := flag.Bool("midi-list", false, "list the MIDI inputs and exit")
	midiCC := flag.Int("midi-cc", 80, "first CC number (snapshot 1); the next three are snapshots 2..4")
	midiCh := flag.Int("midi-channel", 0, "MIDI channel 1..16 to accept (0 = any)")
	settle := flag.Duration("settle", 300*time.Millisecond, "quiet time after a preset change before snapshot 1 is applied")
	flag.Parse()
	if *midiList {
		names, err := midiin.List()
		if err != nil {
			log.Fatal(err)
		}
		for i, n := range names {
			fmt.Printf("%d: %s\n", i, n)
		}
		return
	}
	if *addr != "" && !player.Loopback(*addr) {
		log.Fatalf("-http %q is not a loopback address: the trigger has no authentication", *addr)
	}

	var sess *editor.Session
	var connecting atomic.Bool
	// connectLoop keeps trying until the pedal opens (it may be powered on
	// after the player started, or come back after a power cycle).
	connectLoop := func() {
		if !connecting.CompareAndSwap(false, true) {
			return
		}
		defer connecting.Store(false)
		for {
			time.Sleep(2 * time.Second)
			c, err := openPedal()
			if err != nil {
				continue
			}
			log.Printf("POD HD500 connected")
			sess.Attach(c)
			if err := sess.Refresh(); err != nil {
				log.Printf("first read of the pedal failed: %v", err)
			}
			return
		}
	}
	c, err := openPedal()
	if err != nil {
		log.Printf("offline: %v (retrying every 2 s)", err)
		sess = editor.NewSession(nil, err)
		sess.SetOnLost(func() { connectLoop() })
		go connectLoop()
	} else {
		defer c.Close()
		sess = editor.NewSession(c, nil)
		sess.SetOnLost(func() { log.Printf("POD HD500 lost: reconnecting"); connectLoop() })
		go func() {
			if err := sess.Refresh(); err != nil {
				log.Printf("first read of the pedal failed: %v", err)
			}
		}()
	}
	sess.SetSnapshotPlayer(*dir)

	p := player.New(sess, *settle)
	stop := make(chan struct{})
	go p.Run(stop)

	if *midiMatch != "" || flagSet("midi") {
		name, closeMIDI, err := midiin.Open(*midiMatch, func(m midiin.Message) { p.HandleMIDI(m, *midiCC, *midiCh) })
		if err != nil {
			log.Fatalf("MIDI input: %v", err)
		}
		defer closeMIDI()
		log.Printf("MIDI input %q: CC %d..%d = snapshots 1..4 (channel %s)", name, *midiCC, *midiCC+3, chanText(*midiCh))
	}

	if *addr != "" {
		go func() {
			log.Printf("HTTP trigger on http://%s (POST /snapshot/1..4, GET /status)", *addr)
			if err := http.ListenAndServe(*addr, p.Handler()); err != nil {
				log.Fatal(err)
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	close(stop)
}

// flagSet reports whether the flag was given on the command line (even empty).
func flagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

func chanText(ch int) string {
	if ch == 0 {
		return "any"
	}
	return fmt.Sprint(ch)
}
