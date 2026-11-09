package usb

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	VendorLine6     = 0x0e41
	ProductPODHD500 = 0x414d

	// Line 6 macOS Driver (com_line6_driver_kl6drvmsg) Application IDs
	AppIDPODHD500Edit  uint64 = 0x7D010027 // SendL6SPSIToX4E (spsi[3] == 0x05)
	AppIDPODHD500XEdit uint64 = 0x7D01002D // SendL6SPSIToPodHd500SE (spsi[3] == 0x08)

	// SPSI Address Bytes
	AddrPedal  uint8 = 0x40
	AddrHostX4 uint8 = 0x05 // X4E (POD HD500 Edit)
	AddrHostHD uint8 = 0x08 // PodHd500 (used in protocol.mjs & lpedit-lib)

	// POD HD500 Command Bytes (at byte[7] of the 8-byte SPSI header)
	CmdPresetQuery    uint8 = 0x00
	CmdPresetLoad     uint8 = 0x01
	CmdPresetSet      uint8 = 0x02
	CmdPresetSetAck   uint8 = 0x03
	CmdTypeChange     uint8 = 0x11
	CmdActiveChange   uint8 = 0x13
	CmdTempoParam1    uint8 = 0x14
	CmdSetupChange    uint8 = 0x16
	CmdStatusSet      uint8 = 0x20
	CmdStatusQuery    uint8 = 0x21
	CmdStatusResp     uint8 = 0x22
	CmdAlert          uint8 = 0x23
	CmdPresetSelected uint8 = 0x27
	CmdSetQuery       uint8 = 0x28
	CmdSetLoad        uint8 = 0x29
	CmdSetName        uint8 = 0x2A
	CmdSetNameAck     uint8 = 0x2B
	CmdSetSelected    uint8 = 0x2C
	CmdParamChange    uint8 = 0x2D
	CmdParamMin       uint8 = 0x2E
	CmdParamMax       uint8 = 0x2F
	CmdTempoParam2    uint8 = 0x31
	// CmdExpPedal: expression pedal position, pedal -> host. Payload words:
	// [0]=0, [1]=pedal number (2 seen for the volume pedal), [2]=float32 0.0..1.0.
	// Identified from a hardware capture on 2026-11-03.
	CmdExpPedal uint8 = 0x35

	// Status IDs for CmdStatusQuery (0x21) / CmdStatusResp (0x22) / CmdStatusSet (0x20)
	StatusIDSave          uint32 = 0x00
	StatusIDRefPitch      uint32 = 0x02
	StatusIDCurrentPreset uint32 = 0x08
	StatusIDCurrentSet    uint32 = 0x09

	// Setup IDs for CmdSetupChange (0x16)
	SetupIDTempo       uint32 = 0x17
	SetupIDDT0Topology uint32 = 0x26
	SetupIDDT0Mode     uint32 = 0x27
	SetupIDDT0Class    uint32 = 0x28
	SetupIDDT1Topology uint32 = 0x29
	SetupIDDT1Mode     uint32 = 0x2A
	SetupIDDT1Class    uint32 = 0x2B
	SetupIDMixerLevelA uint32 = 0x2C
	SetupIDMixerLevelB uint32 = 0x2D
	SetupIDMixerPanA   uint32 = 0x2E
	SetupIDMixerPanB   uint32 = 0x2F
	SetupIDCab0ER      uint32 = 0x32
	SetupIDCab1ER      uint32 = 0x33
	SetupIDCab0Mic     uint32 = 0x34
	SetupIDCab1Mic     uint32 = 0x35
	SetupIDInput1Src   uint32 = 0x36
	SetupIDInput2Src   uint32 = 0x37
	SetupIDTrails      uint32 = 0x38
	SetupIDGuitarInZ   uint32 = 0x55

	// PresetSyncSettle: after PRESET_SET_ACK the pedal is still applying the
	// preset (LCD shows "SYNC IN PROGRESS" for < ~1 s, seen 2026-11-03).
	// Reading the edit buffer back sooner can return a partly-updated buffer
	// (once 114 bytes differed at 300 ms), so wait this long before reading.
	PresetSyncSettle = 1500 * time.Millisecond

	CurrentPresetAll uint16 = 0xFFFF
	CurrentSetAll    uint16 = 0xFFFF
	PresetBodySize   int    = 4096
)

// SPSIMessage represents a complete, un-chunked Line 6 POD HD500 message:
// [0..1] uint16LE: size in 32-bit words minus 1 ((totalBytes/4) - 1)
// [2]    uint8:    0x0A
// [3]    uint8:    destAddr (0x40 = to pedal, 0x08/0x05 = from pedal)
// [4]    uint8:    client (0x01 = to pedal, 0x00 = from pedal)
// [5]    uint8:    srcAddr (0x08/0x05 = from host, 0x40 = from pedal)
// [6]    uint8:    0x00
// [7]    uint8:    Cmd (Command ID)
// [8..]  []byte:   Payload (32-bit words)
type SPSIMessage struct {
	Raw      []byte
	Cmd      uint8
	DestAddr uint8
	SrcAddr  uint8
	Payload  []byte
}

// Word returns the i-th 32-bit little-endian word of the payload (starting at byte 8 + 4*i).
func (m *SPSIMessage) Word(i int) uint32 {
	off := i * 4
	if off+4 > len(m.Payload) {
		return 0
	}
	return binary.LittleEndian.Uint32(m.Payload[off : off+4])
}

// FloatWord returns the i-th 32-bit IEEE-754 float of the payload (starting at byte 8 + 4*i).
func (m *SPSIMessage) FloatWord(i int) float32 {
	return math.Float32frombits(m.Word(i))
}

// BuildFrameWithSource constructs an L6SPSI message for the POD HD500 with an explicit source address byte (0x08 or 0x05).
func BuildFrameWithSource(srcAddr uint8, cmd uint8, words []uint32, extraBytes []byte) []byte {
	extraWords := (len(extraBytes) + 3) / 4
	totalWords := 2 + len(words) + extraWords
	totalBytes := totalWords * 4
	wordsMinusOne := uint16(totalWords - 1)

	buf := make([]byte, totalBytes)
	binary.LittleEndian.PutUint16(buf[0:2], wordsMinusOne)
	buf[2] = 0x0A
	buf[3] = AddrPedal
	buf[4] = 0x01
	buf[5] = srcAddr
	buf[6] = 0x00
	buf[7] = cmd

	for i, w := range words {
		binary.LittleEndian.PutUint32(buf[8+i*4:12+i*4], w)
	}
	if len(extraBytes) > 0 {
		copy(buf[8+len(words)*4:], extraBytes)
	}
	return buf
}

// DefaultSourceAddr is the default host address byte used in outgoing frames.
// Note: 0x08 is used by POD HD500/HD500X editors and routed to 0x7D01002D; 0x05 is routed to 0x7D010027.
var DefaultSourceAddr uint8 = AddrHostHD

// BuildFrame constructs an L6SPSI message for the POD HD500 using DefaultSourceAddr.
func BuildFrame(cmd uint8, words ...uint32) []byte {
	return BuildFrameWithSource(DefaultSourceAddr, cmd, words, nil)
}

// ParseSPSI parses an L6SPSI message buffer.
func ParseSPSI(raw []byte) (*SPSIMessage, error) {
	if len(raw) < 8 {
		return nil, fmt.Errorf("SPSI message too short (%d bytes)", len(raw))
	}
	wordsMinusOne := binary.LittleEndian.Uint16(raw[0:2])
	expectedLen := int(wordsMinusOne+1) * 4
	if len(raw) < expectedLen {
		return nil, fmt.Errorf("SPSI message truncated: got %d, expected %d", len(raw), expectedLen)
	}
	payload := make([]byte, expectedLen-8)
	copy(payload, raw[8:expectedLen])
	rawCopy := make([]byte, expectedLen)
	copy(rawCopy, raw[:expectedLen])
	return &SPSIMessage{
		Raw:      rawCopy,
		Cmd:      raw[7],
		DestAddr: raw[3],
		SrcAddr:  raw[5],
		Payload:  payload,
	}, nil
}

// BuildStatusQuery builds Cmd 0x21 (Status Query, e.g., StatusIDCurrentPreset=8, StatusIDCurrentSet=9).
func BuildStatusQuery(statusID uint32) []byte {
	return BuildFrame(CmdStatusQuery, statusID)
}

// BuildStatusSet builds Cmd 0x20 (Status Set, e.g., select preset=8 or select set=9 on hardware).
func BuildStatusSet(statusID, value uint32) []byte {
	return BuildFrame(CmdStatusSet, 0, statusID, value)
}

// BuildPresetQuery builds Cmd 0x00 (Preset Query).
// Pass presetIdx=0xFFFF, setIdx=0xFFFF to query the currently active edit buffer.
func BuildPresetQuery(presetIdx, setIdx uint16) []byte {
	word := (uint32(setIdx) << 16) | uint32(presetIdx)
	return BuildFrame(CmdPresetQuery, word)
}

// BuildPresetSet builds Cmd 0x02 (Preset Set / Apply, 4108 bytes total).
// Pass presetIdx=0xFFFF, setIdx=0xFFFF to update only the live edit buffer without writing flash.
// Pass setIdx=0..7, presetIdx=0..63 to write permanently to flash.
func BuildPresetSet(presetIdx, setIdx uint16, body []byte) ([]byte, error) {
	if len(body) != PresetBodySize {
		return nil, fmt.Errorf("preset body must be %d bytes, got %d", PresetBodySize, len(body))
	}
	dest := (uint32(setIdx) << 16) | uint32(presetIdx)
	return BuildFrameWithSource(DefaultSourceAddr, CmdPresetSet, []uint32{dest}, body), nil
}

// BuildSetQuery builds Cmd 0x28 (Query Setlist Name for setIdx 0..7).
func BuildSetQuery(setIdx uint32) []byte {
	return BuildFrame(CmdSetQuery, setIdx)
}

// BuildSetName builds Cmd 0x2A (Rename Setlist 0..7).
func BuildSetName(setIdx uint32, name string) []byte {
	padded := fmt.Sprintf("%-16s", name)
	if len(padded) > 16 {
		padded = padded[:16]
	}
	return BuildFrameWithSource(DefaultSourceAddr, CmdSetName, []uint32{setIdx}, []byte(padded))
}

// BuildActiveChange builds Cmd 0x13 (Toggle Effect/Amp Block On/Off).
// itemID: 0=Amp A, 1=Cab A, 2=Amp B, 3=Cab B, 4..11=FX 1..8.
func BuildActiveChange(itemID uint32, active bool) []byte {
	var on uint32
	if active {
		on = 1
	}
	return BuildFrame(CmdActiveChange, 0, itemID, on)
}

// BuildTypeChange builds Cmd 0x11 (Change Amp/Cab/FX Model ID).
func BuildTypeChange(itemID, modelType uint32) []byte {
	return BuildFrame(CmdTypeChange, 0, itemID, modelType)
}

// BuildParamChangeFloat builds Cmd 0x2D (Real-time Parameter Change with float32 value).
func BuildParamChangeFloat(itemID, paramID uint32, val float32) []byte {
	return BuildFrame(CmdParamChange, 0, itemID, 1, paramID, math.Float32bits(val))
}

// BuildParamChangeInt builds Cmd 0x2D (Real-time Parameter Change with uint32 value).
func BuildParamChangeInt(itemID, paramID, val uint32) []byte {
	return BuildFrame(CmdParamChange, 0, itemID, 0, paramID, val)
}

// BuildParamMin builds Cmd 0x2E (Parameter Heel / Min Value).
func BuildParamMin(itemID, paramID uint32, val float32) []byte {
	return BuildFrame(CmdParamMin, 0, itemID, 1, paramID, math.Float32bits(val))
}

// BuildParamMax builds Cmd 0x2F (Parameter Toe / Max Value).
func BuildParamMax(itemID, paramID uint32, val float32) []byte {
	return BuildFrame(CmdParamMax, 0, itemID, 1, paramID, math.Float32bits(val))
}

// BuildSetupChangeInt builds Cmd 0x16 (Preset Setup Parameter Change - integer).
func BuildSetupChangeInt(setupID, val uint32) []byte {
	return BuildFrame(CmdSetupChange, 0, 0, setupID, val)
}

// BuildSetupChangeFloat builds Cmd 0x16 (Preset Setup Parameter Change - float32, e.g., Tempo=0x17 or Mixer).
func BuildSetupChangeFloat(setupID uint32, val float32) []byte {
	return BuildFrame(CmdSetupChange, 0, 1, setupID, math.Float32bits(val))
}

// BuildTempoChange builds Cmd 0x16 with SetupIDTempo (0x17).
func BuildTempoChange(bpm float32) []byte {
	return BuildSetupChangeFloat(SetupIDTempo, bpm)
}

// ParseSetName extracts the 16-byte ASCII setlist name from a CmdSetLoad (0x29) message.
func ParseSetName(msg *SPSIMessage) string {
	if len(msg.Payload) < 20 {
		return ""
	}
	raw := msg.Payload[4:20]
	if idx := strings.IndexByte(string(raw), 0); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(string(raw))
}

// ItemName returns a human-readable label for a POD HD500 block ID (0..11).
func ItemName(itemID uint32) string {
	switch itemID {
	case 0:
		return "Amp A"
	case 1:
		return "Cab A"
	case 2:
		return "Amp B"
	case 3:
		return "Cab B"
	case 4, 5, 6, 7, 8, 9, 10, 11:
		return fmt.Sprintf("FX %d", itemID-3)
	default:
		return fmt.Sprintf("Item %d", itemID)
	}
}

// FormatCmd returns a human-readable description of a POD HD500 command byte.
func FormatCmd(cmd uint8) string {
	switch cmd {
	case CmdPresetQuery:
		return "PRESET_QUERY (0x00)"
	case CmdPresetLoad:
		return "PRESET_LOAD (0x01 - 4KB Preset Body)"
	case CmdPresetSet:
		return "PRESET_SET (0x02 - Write Preset)"
	case CmdPresetSetAck:
		return "PRESET_SET_ACK (0x03)"
	case CmdTypeChange:
		return "TYPE_CHANGE (0x11 - Model Change)"
	case CmdActiveChange:
		return "ACTIVE_CHANGE (0x13 - Footswitch/Block Bypass)"
	case CmdTempoParam1:
		return "TEMPO_PARAM_1 (0x14)"
	case CmdSetupChange:
		return "SETUP_CHANGE (0x16 - Tempo/Mixer/InZ/Mic/DT)"
	case CmdStatusSet:
		return "STATUS_SET (0x20 - Switch Preset/Set)"
	case CmdStatusQuery:
		return "STATUS_QUERY (0x21)"
	case CmdStatusResp:
		return "STATUS_RESP (0x22)"
	case CmdAlert:
		return "ALERT (0x23 - Pedal Preset Change Alert)"
	case CmdPresetSelected:
		return "PRESET_SELECTED (0x27 - Footswitch Preset Selected)"
	case CmdSetQuery:
		return "SET_QUERY (0x28)"
	case CmdSetLoad:
		return "SET_LOAD (0x29 - Setlist Name)"
	case CmdSetName:
		return "SET_NAME (0x2A - Rename Setlist)"
	case CmdSetNameAck:
		return "SET_NAME_ACK (0x2B)"
	case CmdSetSelected:
		return "SET_SELECTED (0x2C - Setlist Selected)"
	case CmdParamChange:
		return "PARAM_CHANGE (0x2D - Knob/Parameter Turn)"
	case CmdParamMin:
		return "PARAM_MIN (0x2E - Expression Heel)"
	case CmdParamMax:
		return "PARAM_MAX (0x2F - Expression Toe)"
	case CmdExpPedal:
		return "EXP_PEDAL (0x35 - Expression Pedal Position)"
	case CmdTempoParam2:
		return "TEMPO_PARAM_2 (0x31)"
	default:
		return fmt.Sprintf("UNKNOWN (0x%02X)", cmd)
	}
}
