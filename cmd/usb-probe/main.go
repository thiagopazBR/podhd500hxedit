package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"podhd500-edit/pkg/podhd500"
	"podhd500-edit/pkg/usb"
)

func main() {
	duration := flag.Duration("duration", 8*time.Second, "How long to listen for real-time hardware events (0 = forever until Ctrl+C)")
	toggleTest := flag.Bool("toggle-test", true, "Send a bidirectional test packet (toggle FX block on/off on the physical POD HD500)")
	toggleItem := flag.Int("toggle-item", -1, "Specific item ID (0..11) to toggle (-1 = first non-empty FX block or FX 1)")
	debug := flag.Bool("debug", false, "Log every raw async callback from the Line 6 kext (stderr), before any filtering")
	listenOnlyFlag := flag.Bool("listen-only", false, "Send nothing to the POD HD500: only open the driver pipes and print whatever arrives (Bulk IN)")
	useKext := flag.Bool("kext", false, "Use the Line 6 kext message layer (kl6drvmsg) instead of direct USB; broken on Apple Silicon (sends fail 0xe00002bc)")
	monitor := flag.Float64("monitor", 1.0, "Direct mode: USB playback (Mac audio -> POD) level 0.0..1.0, kept alive while running (-1 = don't touch)")
	flag.Parse()
	usb.Debug = *debug
	listenOnly = *listenOnlyFlag

	fmt.Println("================================================================================")
	fmt.Println("  POD HD500 (0x0e41:0x414d) — macOS Apple Silicon (darwin/arm64) USB Probe PoC")
	fmt.Println("================================================================================")

	var client usb.Client
	if *useKext {
		kc, err := usb.OpenDriverClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERRO] Falha ao abrir driver Line 6 (com_line6_driver_kl6drvmsg): %v\n", err)
			os.Exit(1)
		}
		client = kc
		fmt.Printf("[OK] Conectado ao driver kernel Line6AudioUniversal.kext!\n")
		fmt.Printf("     Pipe Primario (0x7D01002D / addr 0x08): UniqueID = %d\n", kc.UniqueID())
		fmt.Printf("     Pipe Secundario (0x7D010027 / addr 0x05): UniqueID = %d\n", kc.SecondaryUniqueID())
	} else {
		opts := usb.DefaultDirectOptions
		opts.MonitorLevel = float32(*monitor)
		if *monitor < 0 {
			opts.Keepalive = 0
		}
		dc, err := usb.OpenDirectClient(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERRO] Falha ao abrir a interface USB de controle da POD HD500: %v\n", err)
			fmt.Fprintln(os.Stderr, "       Se o erro for 0xe00002c5, rode antes: sudo ./bin/l6-rebind release")
			os.Exit(1)
		}
		client = dc
		fmt.Println("[OK] Interface USB #1 (controle, bulk 0x01/0x81) aberta diretamente em user space.")
		if *monitor >= 0 {
			fmt.Printf("     Playback USB (Mac -> POD) mantido em %.2f enquanto o programa roda.\n", *monitor)
		}
	}
	defer client.Close()
	fmt.Println("--------------------------------------------------------------------------------")

	var currentSet uint32 = 0xFFFFFFFF
	var currentPreset uint32 = 0xFFFFFFFF
	var currentPresetObj *podhd500.Preset

	if listenOnly {
		fmt.Println("[LISTEN-ONLY] Nenhum pacote sera enviado ao pedal; apenas escutando.")
	} else {
		// Try querying Current Set ID (9) with AddrHostHD (0x08), fallback to AddrHostX4 (0x05)
		for _, srcAddr := range []uint8{usb.AddrHostHD, usb.AddrHostX4} {
			usb.DefaultSourceAddr = srcAddr
			fmt.Printf("[HANDSHAKE] Consultando Setlist Atual (StatusQuery id=9, srcAddr=0x%02X)...\n", srcAddr)
			if err := client.SendSPSI(usb.BuildStatusQuery(usb.StatusIDCurrentSet)); err != nil {
				fmt.Printf("  -> Erro ao enviar StatusQuery(9): %v\n", err)
				continue
			}

			msg := waitForCmd(client.RecvChan(), usb.CmdStatusResp, 1200*time.Millisecond)
			if msg != nil {
				currentSet = msg.Word(2)
				fmt.Printf("  -> [RESPOSTA HARDWARE] destAddr=0x%02X srcAddr=0x%02X | Setlist Atual = %d (Set %d)\n",
					msg.DestAddr, msg.SrcAddr, currentSet, currentSet+1)
				break
			}
		}

		// Query Current Preset ID (8)
		fmt.Printf("[HANDSHAKE] Consultando Preset Atual (StatusQuery id=8, srcAddr=0x%02X)...\n", usb.DefaultSourceAddr)
		if sendOK(client, "StatusQuery(8)", usb.BuildStatusQuery(usb.StatusIDCurrentPreset)) {
			if msg := waitForCmd(client.RecvChan(), usb.CmdStatusResp, 1200*time.Millisecond); msg != nil {
				currentPreset = msg.Word(2)
				fmt.Printf("  -> [RESPOSTA HARDWARE] Preset Atual = %d (Slot %s)\n",
					currentPreset, podhd500.SlotLabel(int(currentPreset)))
			}
		}

		// Query Setlist Name if we have currentSet
		if currentSet < 8 {
			if sendOK(client, "SetQuery", usb.BuildSetQuery(currentSet)) {
				if msg := waitForCmd(client.RecvChan(), usb.CmdSetLoad, 1200*time.Millisecond); msg != nil {
					setName := usb.ParseSetName(msg)
					fmt.Printf("  -> [RESPOSTA HARDWARE] Nome do Setlist %d = %q\n", currentSet+1, setName)
				}
			}
		}

		// Query 4096-byte Current Edit Buffer (PresetQuery 0xFFFF, 0xFFFF)
		fmt.Printf("[QUERY] Requisitando Buffer de 4096 bytes do Preset Atual (PresetQuery 0xFFFF, 0xFFFF)...\n")
		if sendOK(client, "PresetQuery(0xFFFF,0xFFFF)", usb.BuildPresetQuery(usb.CurrentPresetAll, usb.CurrentSetAll)) {
			if msg := waitForCmd(client.RecvChan(), usb.CmdPresetLoad, 2500*time.Millisecond); msg != nil {
				fmt.Printf("  -> [RESPOSTA HARDWARE] Recebido PRESET_LOAD (%d bytes SPSI, %d bytes payload)!\n",
					len(msg.Raw), len(msg.Payload))
				if p, err := podhd500.ParsePreset(msg.Payload); err == nil {
					currentPresetObj = p
					printPresetSummary(p, int(currentSet), int(currentPreset))
				} else {
					fmt.Printf("  -> Erro ao fazer parse do preset: %v\n", err)
				}
			} else {
				fmt.Println("  -> Timeout aguardando PRESET_LOAD.")
			}
		}

		// Bidirectional test: Toggle an FX block on the physical POD HD500 and toggle it back
		if *toggleTest && currentPresetObj != nil {
			targetID := *toggleItem
			if targetID < 0 {
				targetID = 4 // Default FX 1
				for id := 4; id <= 11; id++ {
					it := currentPresetObj.Item(id)
					if !it.Empty {
						targetID = id
						break
					}
				}
			}
			it := currentPresetObj.Item(targetID)
			newState := !it.Active
			fmt.Println("--------------------------------------------------------------------------------")
			fmt.Printf("[TESTE BIDIRECIONAL] Alternando bloco %s (%s, FS=%s) de %v -> %v na POD HD500...\n",
				it.Label, it.Model, podhd500.FSNames[it.FS], it.Active, newState)
			if err := client.SendSPSI(usb.BuildActiveChange(uint32(targetID), newState)); err != nil {
				fmt.Printf("  -> Erro ao enviar ActiveChange: %v\n", err)
			} else {
				fmt.Printf("  -> Pacote ACTIVE_CHANGE (0x13) enviado com sucesso! (Verifique o visor da POD HD500)\n")
				time.Sleep(1500 * time.Millisecond)
				fmt.Printf("[TESTE BIDIRECIONAL] Restaurando bloco %s (%s) para %v...\n",
					it.Label, it.Model, it.Active)
				sendOK(client, "ActiveChange(restore)", usb.BuildActiveChange(uint32(targetID), it.Active))
			}
		}
	} // !listenOnly

	fmt.Println("--------------------------------------------------------------------------------")
	if *duration > 0 {
		fmt.Printf("[ESCUTA EM TEMPO REAL] Escutando eventos da POD HD500 por %v...\n", *duration)
	} else {
		fmt.Println("[ESCUTA EM TEMPO REAL] Escutando eventos da POD HD500 (Pressione Ctrl+C para sair)...")
	}
	fmt.Println("  (Pise em um Footswitch, gire um Knob ou aperte TAP na pedaleira!)")
	fmt.Println("--------------------------------------------------------------------------------")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	var timeoutCh <-chan time.Time
	if *duration > 0 {
		timeoutCh = time.After(*duration)
	}

	for {
		select {
		case <-sigCh:
			fmt.Println("\n[ENCERRANDO] Fechando conexao com a POD HD500...")
			return
		case <-timeoutCh:
			fmt.Println("\n[CONCLUIDO] Tempo de escuta finalizado com sucesso.")
			return
		case msg := <-client.RecvChan():
			printHardwareEvent(client, msg, currentPresetObj)
		}
	}
}

// listenOnly suppresses every outgoing packet, including the automatic
// re-query of the edit buffer when the pedal reports a preset change.
var listenOnly bool

// sendOK sends a frame and reports any driver error instead of hiding it.
func sendOK(client usb.Client, label string, frame []byte) bool {
	if listenOnly {
		return false
	}
	if err := client.SendSPSI(frame); err != nil {
		fmt.Printf("  -> [ERRO ENVIO] %s: %v\n", label, err)
		return false
	}
	return true
}

func waitForCmd(ch <-chan *usb.SPSIMessage, expectedCmd uint8, timeout time.Duration) *usb.SPSIMessage {
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return nil
		case msg := <-ch:
			if msg == nil {
				continue
			}
			if msg.Cmd == expectedCmd {
				return msg
			}
			fmt.Printf("  [EVENTO ASSINCRONO DURANTE HANDSHAKE] %s (len=%d, dest=0x%02X, src=0x%02X)\n",
				usb.FormatCmd(msg.Cmd), len(msg.Raw), msg.DestAddr, msg.SrcAddr)
		}
	}
}

func printPresetSummary(p *podhd500.Preset, setIdx, presetIdx int) {
	fmt.Println("================================================================================")
	fmt.Printf("  PRESET CARREGADO DA POD HD500 FISICA:\n")
	fmt.Printf("  Nome:      %q\n", p.Name())
	if setIdx >= 0 && presetIdx >= 0 {
		fmt.Printf("  Posicao:   Setlist %d | Slot %s (indice %d)\n", setIdx+1, podhd500.SlotLabel(presetIdx), presetIdx)
	}
	fmt.Printf("  Tempo:     %.1f BPM | Guitar In-Z: %s\n", p.Tempo(), p.GuitarInZ())
	fmt.Println("--------------------------------------------------------------------------------")
	for _, it := range p.Items() {
		state := "OFF"
		if it.Active {
			state = "ON "
		}
		if it.Empty {
			state = "---"
		}
		fsName := "—"
		if int(it.FS) < len(podhd500.FSNames) {
			fsName = podhd500.FSNames[it.FS]
		}
		laneName := fmt.Sprintf("L%d", it.Lane)
		if int(it.Lane) < len(podhd500.LaneNames) {
			laneName = podhd500.LaneNames[it.Lane]
		}
		based := ""
		if it.BasedOn != "" {
			based = fmt.Sprintf(" (%s)", it.BasedOn)
		}
		fmt.Printf("  [%2d] %-5s | [%s] %-22s%-28s | FS: %-7s | Lane: %-11s | Params: %d\n",
			it.ID, it.Label, state, it.Model, based, fsName, laneName, len(it.Params))
	}
	fmt.Println("================================================================================")
}

func printHardwareEvent(client usb.Client, msg *usb.SPSIMessage, p *podhd500.Preset) {
	ts := time.Now().Format("15:04:05.000")
	switch msg.Cmd {
	case usb.CmdActiveChange:
		itemID := msg.Word(1)
		active := msg.Word(2) != 0
		modelName := usb.ItemName(itemID)
		if p != nil && itemID < 12 {
			it := p.Item(int(itemID))
			modelName = fmt.Sprintf("%s (%s)", it.Label, it.Model)
		}
		fmt.Printf("[%s] [FOOTSWITCH / BYPASS] %s -> Active = %v\n", ts, modelName, active)

	case usb.CmdParamChange:
		itemID := msg.Word(1)
		vtype := msg.Word(2)
		paramID := msg.Word(3)
		valFloat := msg.FloatWord(4)
		valUint := msg.Word(4)
		itemLabel := usb.ItemName(itemID)
		var modelType uint32
		if p != nil && itemID < 12 {
			it := p.Item(int(itemID))
			modelType = it.Type
			itemLabel = fmt.Sprintf("%s (%s)", it.Label, it.Model)
		}
		pName := podhd500.LookupParamName(modelType, paramID)
		if vtype == 1 {
			fmt.Printf("[%s] [KNOB / PARAM] %s | %s (0x%08X) = %.3f (%.0f%%)\n",
				ts, itemLabel, pName, paramID, valFloat, valFloat*100)
		} else {
			fmt.Printf("[%s] [KNOB / PARAM] %s | %s (0x%08X) = %d (0x%X)\n",
				ts, itemLabel, pName, paramID, valUint, valUint)
		}

	case usb.CmdTypeChange:
		itemID := msg.Word(1)
		newType := msg.Word(2)
		m := podhd500.LookupModel(newType)
		mName := fmt.Sprintf("0x%08X", newType)
		if m != nil {
			mName = m.Name
		}
		fmt.Printf("[%s] [MODEL CHANGE] %s -> %s\n", ts, usb.ItemName(itemID), mName)

	case usb.CmdSetupChange:
		vtype := msg.Word(1)
		setupID := msg.Word(2)
		sEntry, ok := podhd500.LookupSetup(setupID)
		sName := fmt.Sprintf("SetupID 0x%02X", setupID)
		if ok && sEntry.Name != "" {
			sName = sEntry.Name
		} else if setupID == usb.SetupIDTempo {
			sName = "Tempo BPM"
		}
		if vtype == 1 {
			fmt.Printf("[%s] [SETUP / TAP TEMPO] %s (0x%02X) = %.2f\n", ts, sName, setupID, msg.FloatWord(3))
		} else {
			fmt.Printf("[%s] [SETUP] %s (0x%02X) = %d\n", ts, sName, setupID, msg.Word(3))
		}

	case usb.CmdPresetSelected:
		presetID := msg.Word(0)
		fmt.Printf("[%s] [PRESET SELECTED] Pedal trocou para o Slot %s (indice %d)!\n",
			ts, podhd500.SlotLabel(int(presetID)), presetID)
		// No PresetQuery here: the pedal pushes PRESET_LOAD (0x01) by itself
		// right after ALERT/SET_SELECTED/PRESET_SELECTED (seen on hardware).

	case usb.CmdSetSelected:
		setID := msg.Word(0)
		fmt.Printf("[%s] [SETLIST SELECTED] Pedal trocou para o Setlist %d!\n", ts, setID+1)

	case usb.CmdAlert:
		fmt.Printf("[%s] [ALERT 0x23] Pedal vai trocar de preset (o PRESET_LOAD chega em seguida)\n", ts)

	case usb.CmdExpPedal:
		fmt.Printf("[%s] [PEDAL EXP %d] posicao = %.3f (%.0f%%)\n", ts, msg.Word(1), msg.FloatWord(2), msg.FloatWord(2)*100)

	case usb.CmdPresetLoad:
		if newP, err := podhd500.ParsePreset(msg.Payload); err == nil {
			if p != nil {
				*p = *newP
			}
			fmt.Printf("[%s] [PRESET_LOAD] Novo preset carregado: %q (%.1f BPM)\n", ts, newP.Name(), newP.Tempo())
		}

	default:
		fmt.Printf("[%s] [PACOTE SPSI] %s | len=%d | dest=0x%02X src=0x%02X | raw=%x\n",
			ts, usb.FormatCmd(msg.Cmd), len(msg.Raw), msg.DestAddr, msg.SrcAddr, msg.Raw)
	}
}
