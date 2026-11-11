//go:build darwin

// l6-rebind restarts the Line 6 bulk pipe driver (com_line6_driver_bulkpipecomm)
// for the POD HD500 after the rest of the Line 6 driver stack is up.
//
// Why: in Line6AudioUniversal.kext, PODX3Mac::ConfigureBulk (which binds the
// bulk endpoints to the HD500 feature handler) is only ever called from
// bulkpipecomm::start, and only if com_line6_driver_kl6drvmsg already exists.
// bulkpipecomm (USB interface 1) starts before com_line6_audio creates
// kl6drvmsg, so the binding never happens and every SPSI send fails with
// 0xe00002bc.
//
// A user-space rematch is impossible on current macOS: IOUSBHostInterface
// returns kIOReturnUnsupported for IOServiceRequestProbe, and re-sending
// personalities (kIOCatalogAddDrivers) requires an Apple entitlement. So the
// plan is to release interface 1 from the kext and drive the bulk endpoints
// directly from user space, leaving audio (interface 0) on the kext.
//
// Usage:
//
//	l6-rebind status          # no privileges needed, changes nothing
//	l6-rebind open            # try to open a user client on interface 1 (no root)
//	sudo l6-rebind release    # terminate bulkpipecomm so interface 1 is free
//	sudo l6-rebind install    # ONE TIME: a launchd daemon runs "release" by itself
//	                          # whenever the pedal is plugged in or powered on (polls every 2 s)
//	sudo l6-rebind uninstall  # removes the daemon
//	l6-rebind install -n      # dry run: prints what install would do (no root)
//
// Replugging the POD HD500 restores the original driver state (the daemon, if
// installed, releases the interface again).
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	vendorLine6  = 0x0e41
	productHD500 = 0x414d
	bulkIface    = 1

	classBulk   = "com_line6_driver_bulkpipecomm"
	classDrvMsg = "com_line6_driver_kl6drvmsg"

	kIOCatalogServiceTerminate = 3 // IOKitServer.h: ModuleUnload=1, ModuleTerminate=2, ServiceTerminate=3

	kCFNumberSInt32Type      = 3
	kCFStringEncodingUTF8    = 0x08000100
	kIORegistryIterateParent = 2
)

var (
	ioServiceMatching             func(name *byte) uintptr
	ioServiceGetMatchingService   func(mainPort uint32, matching uintptr) uint32
	ioServiceGetMatchingServices  func(mainPort uint32, matching uintptr, iter *uint32) int32
	ioIteratorNext                func(iter uint32) uint32
	ioObjectRelease               func(obj uint32) int32
	ioObjectGetClass              func(obj uint32, name *byte) int32
	ioRegistryEntryGetRegistryID  func(entry uint32, id *uint64) int32
	ioRegistryEntryGetParentEntry func(entry uint32, plane *byte, parent *uint32) int32
	ioRegistryEntryGetChildIter   func(entry uint32, plane *byte, iter *uint32) int32
	ioServiceRequestProbe         func(service uint32, options uint32) int32
	ioCatalogueTerminate          func(mainPort uint32, flag uint32, name *byte) int32
	ioServiceOpen                 func(service uint32, task uint32, typ uint32, conn *uint32) int32
	ioServiceClose                func(conn uint32) int32
	machTaskSelf                  func() uint32

	cfNumberCreate           func(alloc uintptr, typ int, valuePtr unsafe.Pointer) uintptr
	cfStringCreateWithCStr   func(alloc uintptr, s *byte, enc uint32) uintptr
	cfDictionarySetValue     func(dict uintptr, key uintptr, value uintptr)
	cfRelease                func(obj uintptr)
	cfDictionaryCreateMutabl func(alloc uintptr, cap int, keyCB uintptr, valCB uintptr) uintptr
	kCFTypeDictKeyCallBacks  uintptr
	kCFTypeDictValCallBacks  uintptr
)

func cstr(s string) *byte { b := append([]byte(s), 0); return &b[0] }

func gostr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func initIOKit() {
	iokit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	must(err)
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	must(err)
	purego.RegisterLibFunc(&ioServiceMatching, iokit, "IOServiceMatching")
	purego.RegisterLibFunc(&ioServiceGetMatchingService, iokit, "IOServiceGetMatchingService")
	purego.RegisterLibFunc(&ioServiceGetMatchingServices, iokit, "IOServiceGetMatchingServices")
	purego.RegisterLibFunc(&ioIteratorNext, iokit, "IOIteratorNext")
	purego.RegisterLibFunc(&ioObjectRelease, iokit, "IOObjectRelease")
	purego.RegisterLibFunc(&ioObjectGetClass, iokit, "IOObjectGetClass")
	purego.RegisterLibFunc(&ioRegistryEntryGetRegistryID, iokit, "IORegistryEntryGetRegistryEntryID")
	purego.RegisterLibFunc(&ioRegistryEntryGetParentEntry, iokit, "IORegistryEntryGetParentEntry")
	purego.RegisterLibFunc(&ioRegistryEntryGetChildIter, iokit, "IORegistryEntryGetChildIterator")
	purego.RegisterLibFunc(&ioServiceRequestProbe, iokit, "IOServiceRequestProbe")
	purego.RegisterLibFunc(&ioCatalogueTerminate, iokit, "IOCatalogueTerminate")
	purego.RegisterLibFunc(&ioServiceOpen, iokit, "IOServiceOpen")
	purego.RegisterLibFunc(&ioServiceClose, iokit, "IOServiceClose")
	libSys, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	must(err)
	purego.RegisterLibFunc(&machTaskSelf, libSys, "mach_task_self")

	purego.RegisterLibFunc(&cfNumberCreate, cf, "CFNumberCreate")
	purego.RegisterLibFunc(&cfStringCreateWithCStr, cf, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&cfDictionarySetValue, cf, "CFDictionarySetValue")
	purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	purego.RegisterLibFunc(&cfDictionaryCreateMutabl, cf, "CFDictionaryCreateMutable")
	k, err := purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks")
	must(err)
	kCFTypeDictKeyCallBacks = k
	v, err := purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks")
	must(err)
	kCFTypeDictValCallBacks = v
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERRO]", err)
		os.Exit(1)
	}
}

func className(obj uint32) string {
	var b [128]byte
	ioObjectGetClass(obj, &b[0])
	return gostr(b[:])
}

func regID(obj uint32) uint64 {
	var id uint64
	ioRegistryEntryGetRegistryID(obj, &id)
	return id
}

// findService returns the first service of the given class (caller releases).
func findService(class string) uint32 {
	return ioServiceGetMatchingService(0, ioServiceMatching(cstr(class)))
}

// findBulkInterface locates the HD500 IOUSBHostInterface #1 by its properties.
func findBulkInterface() uint32 {
	match := ioServiceMatching(cstr("IOUSBHostInterface"))
	// IOServiceMatching returns a mutable dictionary; add property filters.
	props := cfDictionaryCreateMutabl(0, 0, kCFTypeDictKeyCallBacks, kCFTypeDictValCallBacks)
	setNum := func(key string, v int32) {
		k := cfStringCreateWithCStr(0, cstr(key), kCFStringEncodingUTF8)
		n := cfNumberCreate(0, kCFNumberSInt32Type, unsafe.Pointer(&v))
		cfDictionarySetValue(props, k, n)
		cfRelease(k)
		cfRelease(n)
	}
	setNum("idVendor", vendorLine6)
	setNum("idProduct", productHD500)
	setNum("bInterfaceNumber", bulkIface)
	pk := cfStringCreateWithCStr(0, cstr("IOPropertyMatch"), kCFStringEncodingUTF8)
	cfDictionarySetValue(match, pk, props)
	cfRelease(pk)
	cfRelease(props)
	return ioServiceGetMatchingService(0, match) // consumes match
}

func children(entry uint32) []string {
	var iter uint32
	if ioRegistryEntryGetChildIter(entry, cstr("IOService"), &iter) != 0 {
		return nil
	}
	defer ioObjectRelease(iter)
	var out []string
	for {
		c := ioIteratorNext(iter)
		if c == 0 {
			return out
		}
		out = append(out, fmt.Sprintf("%s (id 0x%x)", className(c), regID(c)))
		ioObjectRelease(c)
	}
}

func status() {
	fmt.Println("=== Estado do driver Line 6 (POD HD500) ===")
	for _, cls := range []string{classBulk, "com_line6_audio", classDrvMsg} {
		s := findService(cls)
		if s == 0 {
			fmt.Printf("  %-32s : AUSENTE\n", cls)
			continue
		}
		fmt.Printf("  %-32s : id 0x%x\n", cls, regID(s))
		ioObjectRelease(s)
	}
	iface := findBulkInterface()
	if iface == 0 {
		fmt.Println("  Interface USB #1 do HD500       : NAO ENCONTRADA (pedal desconectado?)")
		return
	}
	defer ioObjectRelease(iface)
	ch := children(iface)
	fmt.Printf("  Interface USB #1 do HD500       : id 0x%x, clientes: %v\n", regID(iface), ch)

	b, d := findService(classBulk), findService(classDrvMsg)
	if b != 0 && d != 0 {
		if regID(b) < regID(d) {
			fmt.Println("  -> bulkpipecomm foi criado ANTES do kl6drvmsg: ConfigureBulk provavelmente NAO rodou (envio SPSI vai falhar).")
		} else {
			fmt.Println("  -> bulkpipecomm foi criado DEPOIS do kl6drvmsg: ConfigureBulk deve ter rodado.")
		}
	}
	if b != 0 {
		ioObjectRelease(b)
	}
	if d != 0 {
		ioObjectRelease(d)
	}
}

// tryOpen attempts to create a user client on interface 1 (what a user-space
// USB driver would do first). Reports whether the kernel allows it.
func tryOpen() {
	iface := findBulkInterface()
	if iface == 0 {
		must(fmt.Errorf("interface USB #1 do HD500 nao encontrada"))
	}
	defer ioObjectRelease(iface)
	fmt.Printf("  euid=%d | clientes da interface #1: %v\n", os.Geteuid(), children(iface))
	var conn uint32
	kr := ioServiceOpen(iface, machTaskSelf(), 0, &conn)
	fmt.Printf("  IOServiceOpen(interface #1, type 0) = 0x%08x (conn=0x%x)\n", uint32(kr), conn)
	if kr == 0 && conn != 0 {
		ioServiceClose(conn)
		fmt.Println("  -> user client criado: o acesso de user space a interface #1 e permitido.")
	}
}

func release() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "[ERRO] 'release' precisa de root: rode com sudo.")
		os.Exit(1)
	}
	b := findService(classBulk)
	if b == 0 {
		fmt.Println("bulkpipecomm ja nao esta ativo; interface #1 livre.")
		return
	}
	ioObjectRelease(b)

	fmt.Println("[1/2] Encerrando com_line6_driver_bulkpipecomm (IOCatalogueTerminate/ServiceTerminate)...")
	kr := ioCatalogueTerminate(0, kIOCatalogServiceTerminate, cstr(classBulk))
	fmt.Printf("  IOCatalogueTerminate = 0x%08x\n", uint32(kr))
	if kr != 0 {
		fmt.Println("  -> O kernel recusou o encerramento. Nada foi alterado.")
		os.Exit(2)
	}
	time.Sleep(500 * time.Millisecond)

	fmt.Println("[2/2] Verificando...")
	ok := true
	if s := findService(classBulk); s != 0 {
		ioObjectRelease(s)
		fmt.Println("  -> bulkpipecomm AINDA presente (inesperado).")
		ok = false
	}
	for _, cls := range []string{"com_line6_audio", classDrvMsg} {
		s := findService(cls)
		if s == 0 {
			fmt.Printf("  -> ATENCAO: %s sumiu (audio afetado). Replugue a POD.\n", cls)
			ok = false
			continue
		}
		ioObjectRelease(s)
	}
	if ok {
		fmt.Println("  -> OK: interface #1 liberada; audio (com_line6_audio/kl6drvmsg) intacto.")
		fmt.Println("     Para voltar ao estado original: replugue a POD.")
	}
}

func main() {
	initIOKit()
	cmd := "status"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "status":
		status()
	case "open":
		tryOpen()
	case "release":
		release()
		fmt.Println()
		status()
	case "install":
		install(len(os.Args) > 2 && (os.Args[2] == "-n" || os.Args[2] == "--dry-run"))
	case "uninstall":
		uninstall()
	case "watch":
		watch()
	case "daemon":
		daemon()
	default:
		fmt.Fprintf(os.Stderr, "uso: %s [status|open|release|install [-n]|uninstall]\n", os.Args[0])
		os.Exit(1)
	}
}

// ---- daemon: release the interface automatically (no sudo after a one-time install) ----

const (
	daemonLabel   = "com.hd500edit.l6-rebind"
	daemonPlist   = "/Library/LaunchDaemons/" + daemonLabel + ".plist"
	daemonDir     = "/usr/local/libexec/hd500"
	daemonBinary  = daemonDir + "/l6-rebind"
	daemonLogPath = "/var/log/hd500-l6-rebind.log"
)

// daemonPlistXML is the launchd job: a root process kept alive that runs
// "<binary> watch". It polls every 2 s and, when the pedal's USB interface is
// held by the Line 6 bulkpipecomm driver, releases it once. It does nothing
// else. (An IOKit launch event was tried first: launchd kept respawning the
// job every 10 s because the event stream is never consumed.)
func daemonPlistXML() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>watch</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, daemonLabel, daemonBinary, daemonLogPath, daemonLogPath)
}

func launchctl(args ...string) error {
	out, err := exec.Command("/bin/launchctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func install(dry bool) {
	self, err := os.Executable()
	must(err)
	self, err = filepath.EvalSymlinks(self)
	must(err)
	fmt.Println("Instalacao do daemon (rodar UMA vez, com sudo):")
	fmt.Printf("  1. copia %s -> %s (root:wheel, 0755)\n", self, daemonBinary)
	fmt.Printf("  2. grava %s (root:wheel, 0644)\n", daemonPlist)
	fmt.Printf("  3. launchctl bootstrap system %s\n", daemonPlist)
	fmt.Println("  O daemon (root) verifica a cada 2 s se o POD HD500 esta com a interface #1 presa")
	fmt.Println("  pelo driver Line 6 e so entao executa 'release'; log em " + daemonLogPath + ".")
	if dry {
		fmt.Println("\n--- plist ---")
		fmt.Print(daemonPlistXML())
		return
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "[ERRO] 'install' precisa de root: rode com sudo.")
		os.Exit(1)
	}
	// The root-run binary lives in a root-owned directory, so a normal user
	// cannot swap it for something else.
	must(os.MkdirAll(daemonDir, 0o755))
	must(os.Chown(daemonDir, 0, 0))
	must(os.Chmod(daemonDir, 0o755))
	must(copyRootFile(self, daemonBinary, 0o755))
	must(os.WriteFile(daemonPlist, []byte(daemonPlistXML()), 0o644))
	must(os.Chown(daemonPlist, 0, 0))
	must(os.Chmod(daemonPlist, 0o644))
	_ = launchctl("bootout", "system/"+daemonLabel) // reinstall: ignore "not loaded"
	must(launchctl("bootstrap", "system", daemonPlist))
	fmt.Println("\n-> OK: instalado. Ligue/plugue a POD HD500: a interface #1 sera liberada sozinha.")
	fmt.Println("   Acompanhe em: tail -f " + daemonLogPath)
	fmt.Println("   Para remover: sudo " + daemonBinary + " uninstall")
}

func copyRootFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chown(tmp, 0, 0); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func uninstall() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "[ERRO] 'uninstall' precisa de root: rode com sudo.")
		os.Exit(1)
	}
	if err := launchctl("bootout", "system/"+daemonLabel); err != nil {
		fmt.Println("  (daemon nao estava carregado:", err, ")")
	}
	for _, p := range []string{daemonPlist, daemonBinary} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			fmt.Println("  nao removi", p+":", err)
		}
	}
	_ = os.Remove(daemonDir)
	fmt.Println("-> OK: daemon removido. Volte a usar: sudo ./bin/l6-rebind release")
}

func logf(format string, a ...any) {
	fmt.Printf("%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, a...))
}

// daemon is the name the first installed plist used; it behaves as watch.
func daemon() { watch() }

// watch polls every 2 s. When the pedal is present and the Line 6
// bulkpipecomm driver holds interface #1, it waits for the audio stack to come
// up (at most 20 s), then releases the interface once. State changes are
// logged; idle polls are silent.
func watch() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "[ERRO] 'watch' roda como root (launchd).")
		os.Exit(1)
	}
	logf("vigiando o POD HD500 (a cada 2 s)")
	var firstSeen, failedUntil time.Time
	for {
		time.Sleep(2 * time.Second)
		iface := findBulkInterface()
		if iface == 0 {
			if !firstSeen.IsZero() {
				logf("o POD HD500 saiu da USB")
				firstSeen = time.Time{}
			}
			continue
		}
		ioObjectRelease(iface)
		b := findService(classBulk)
		if b == 0 { // free already, or the driver has not started yet
			firstSeen = time.Time{}
			continue
		}
		ioObjectRelease(b)
		if firstSeen.IsZero() {
			firstSeen = time.Now()
			logf("POD HD500 detectada com o driver Line 6 na interface #1")
		}
		if time.Now().Before(failedUntil) {
			continue
		}
		audio, msg := findService("com_line6_audio"), findService(classDrvMsg)
		up := audio != 0 && msg != 0
		if audio != 0 {
			ioObjectRelease(audio)
		}
		if msg != 0 {
			ioObjectRelease(msg)
		}
		if !up && time.Since(firstSeen) < 20*time.Second {
			continue
		}
		if !up {
			logf("pilha de audio Line 6 nao subiu em 20 s; liberando mesmo assim")
		}
		kr := ioCatalogueTerminate(0, kIOCatalogServiceTerminate, cstr(classBulk))
		logf("IOCatalogueTerminate(%s) = 0x%08x", classBulk, uint32(kr))
		time.Sleep(500 * time.Millisecond)
		if s := findService(classBulk); s != 0 {
			ioObjectRelease(s)
			logf("bulkpipecomm AINDA presente (inesperado); nova tentativa em 30 s")
			failedUntil = time.Now().Add(30 * time.Second)
			continue
		}
		logf("OK: interface #1 liberada")
		firstSeen = time.Time{}
	}
}
