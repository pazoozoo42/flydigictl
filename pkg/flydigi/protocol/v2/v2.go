// Package v2 implements the "NewXInput" Flydigi protocol used by controllers
// with Flydigi's own USB vendor ID (Vader 5 Pro, Apex 5, Apex 6, ...).
//
// Commands and replies are 32-byte HID reports on the vendor interface
// (usage page 0xFFA0):
//
//	[5A][A5][cmd][len][payload...][crc]
//
// where len = len(payload)+2 and crc = sum(bytes[2 : 2+len]) mod 256, i.e.
// the sum of cmd, len and payload. The report is preceded by a report ID byte
// (0 for unnumbered reports) when written through hidraw.
//
// Command and reply layouts were derived from Flydigi Space Station 4 and
// SDL's HIDAPI Flydigi driver.
package v2

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/pipe01/flydigictl/pkg/flydigi/protocol"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/hidraw"

	"github.com/rs/zerolog/log"
)

const (
	// VendorID is Flydigi's own USB vendor ID ("USB_VENDOR_FLYDIGI_V2" in SDL).
	VendorID = 0x37d7
	// UsagePage of the vendor HID interface carrying the protocol.
	UsagePage = 0xFFA0

	reportSize = 32
)

const (
	magic1 = 0x5A
	magic2 = 0xA5
)

// Command IDs.
const (
	CmdGetInfo             = 0x01 // HeartBeat / device info
	CmdReadHardwareFuncs   = 0x03
	CmdReadRawDataStatus   = 0x10 // includes third-party takeover flag
	CmdEnableRawData       = 0x11 // sets third-party takeover flag
	CmdAcquireController   = 0x1C
	CmdInputReport         = 0xEF
	CmdCalibrationADC      = 0xF0
	CmdOriginDataReport    = 0xF7
	CmdEnableJoystickAutoC = 0x13
)

type protocolV2 struct {
	dev  *hidraw.Device
	info hidraw.DeviceInfo

	inReportID, outReportID byte

	msgch chan protocol.Message

	writeMu sync.Mutex
}

// IsV2Device reports whether the given hidraw device is the vendor interface of a V2 controller.
func IsV2Device(d hidraw.DeviceInfo) bool {
	// Flydigi Space: VendorId == 0x37d7 && UsagePage == 0xFFA0 && ProductId>>12 == 2 && (ProductId & 0xFF00) != 0x2800
	return d.VendorID == VendorID && d.UsagePage() == UsagePage && d.ProductID>>12 == 2 && d.ProductID&0xFF00 != 0x2800
}

func Open() (protocol.Protocol, error) {
	devs, err := hidraw.Find(IsV2Device)
	if err != nil {
		return nil, fmt.Errorf("enumerate devices: %w", err)
	}

	if len(devs) == 0 {
		return nil, protocol.ErrGamepadNotPresent
	}

	info := devs[0]

	dev, err := hidraw.Open(info.Path)
	if err != nil {
		return nil, fmt.Errorf("open hid device: %w", err)
	}

	first, last := info.ReportIDs()

	log.Debug().
		Str("path", info.Path).
		Str("name", info.Name).
		Uint16("pid", info.ProductID).
		Uint8("in_report_id", first).
		Uint8("out_report_id", last).
		Msg("opened v2 hid device")

	p := &protocolV2{
		dev:         dev,
		info:        info,
		inReportID:  first,
		outReportID: last,
		msgch:       make(chan protocol.Message, 10),
	}

	go p.readLoop()

	return p, nil
}

func (p *protocolV2) Close() error {
	return p.dev.Close()
}

func (p *protocolV2) Version() protocol.Version {
	return protocol.VersionV2
}

func (p *protocolV2) USBDevicePath() string {
	return p.info.USBDevice
}

func (p *protocolV2) Messages() <-chan protocol.Message {
	return p.msgch
}

func (p *protocolV2) readLoop() {
	buf := make([]byte, 64)

	defer close(p.msgch)

	for {
		n, err := p.dev.Read(buf)
		if err != nil {
			break
		}

		data := buf[:n]

		// Strip the report ID if present
		if len(data) > 0 && data[0] != magic1 && (p.inReportID == 0 || data[0] == p.inReportID) {
			data = data[1:]
		}

		if len(data) < reportSize || data[0] != magic1 || data[1] != magic2 {
			continue
		}

		if msg, ok := p.parse(data); ok {
			p.msgch <- msg
		}
	}
}

func nibbleVersion(hi, lo byte) string {
	if hi == 0 && lo == 0 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", hi>>4, hi&0xF, lo>>4, lo&0xF)
}

func (p *protocolV2) parse(data []byte) (protocol.Message, bool) {
	cmd := data[2]

	switch cmd {
	case CmdInputReport, CmdOriginDataReport:
		return nil, false

	case CmdGetInfo:
		return parseInfo(data)

	case CmdReadRawDataStatus:
		return protocol.MessageTakeoverStatus{
			XInputEnabled:  data[5] == 1,
			RawDataEnabled: data[6] == 1,
			Enabled:        data[9] == 1,
			ControlBy:      strings.TrimRight(string(data[10:30]), "\x00"),
		}, true

	default:
		log.Debug().Uint8("cmd", cmd).Hex("data", data).Msg("got v2 reply")
		return protocol.MessageAck{Command: cmd, Data: append([]byte(nil), data...)}, true
	}
}

// parseInfo mirrors HeartBeatControllerCommandNewXInput.ParseAckData from Flydigi Space Station 4.
func parseInfo(data []byte) (protocol.Message, bool) {
	// data[3] = total packets, data[4] = packet index (only the first one carries the fields we need)
	multi := data[4] < data[3]
	if multi && data[4] != 0 {
		return nil, false
	}

	i := 4
	if multi {
		i = 5
	}

	next := func() byte {
		b := data[i]
		i++
		return b
	}

	msg := protocol.MessageGamePadInfo{}
	ext := &protocol.ExtendedInfo{}

	msg.DeviceID = next()
	msg.ConnectionType = next()

	mac := []byte{next(), next(), next(), next()}
	msg.DeviceMac = mac

	battery := next()
	msg.Battery = battery
	ext.BatteryLevel = int(battery & 0xF)
	switch battery >> 4 {
	case 0:
		ext.BatteryState = protocol.BatteryStateOnBattery
	case 1:
		ext.BatteryState = protocol.BatteryStateCharging
	case 2:
		ext.BatteryState = protocol.BatteryStateCharged
		ext.BatteryLevel = 5
	}

	ext.ChipType = next() & 0xF
	msg.CPUType = ext.ChipType
	msg.MotionSensorType = next() & 0xF
	next() // reserved

	msg.FW_H = data[i]
	msg.FW_L = data[i+1]
	ext.FirmwareVersion = nibbleVersion(next(), next())
	ext.DongleVersion = nibbleVersion(next(), next())
	ext.SIVersion = nibbleVersion(next(), next())
	ext.TriggerVersion = nibbleVersion(next(), next())
	ext.ScreenVersion = nibbleVersion(next(), next())
	ext.ADCVersion = nibbleVersion(next(), next())
	ext.RFVersion = nibbleVersion(next(), next())

	msg.Extended = ext

	return msg, true
}

func (p *protocolV2) Send(ctx context.Context, cmd protocol.Command) error {
	switch cmd := cmd.(type) {
	case protocol.CommandGetDeviceInfo:
		return p.write(CmdGetInfo, nil)

	case protocol.CommandGetDongleVersion:
		// Dongle version is part of the device info reply
		return nil

	case protocol.CommandGetTakeover:
		return p.write(CmdReadRawDataStatus, nil)

	case protocol.CommandSetTakeover:
		// EnableRawDataTransportInCommand: [controllerData, rawData, keyboardData, mouseData, thirdPartyControl]
		// 0xFF = leave unchanged
		enable := byte(0)
		if cmd.Enable {
			enable = 1
		}
		return p.write(CmdEnableRawData, []byte{0xFF, 0xFF, 0xFF, 0xFF, enable})

	case protocol.CommandCalibrate:
		// CalibrationAdcCommandNewXInput: [1 = start, 2 = finish]
		status := byte(2)
		if cmd.Start {
			status = 1
		}
		return p.write(CmdCalibrationADC, []byte{status})

	case protocol.CommandReadConfig, protocol.CommandReadLEDConfig, protocol.CommandSendConfig, protocol.CommandSendLEDConfig:
		return protocol.ErrUnsupported

	default:
		return protocol.ErrUnknownCommand
	}
}

// Packet builds a command report (without the leading report ID byte).
func Packet(cmd byte, payload []byte) []byte {
	if len(payload) > reportSize-6 {
		panic("v2: payload too long")
	}

	pkt := make([]byte, reportSize)
	pkt[0] = magic1
	pkt[1] = magic2
	pkt[2] = cmd
	pkt[3] = byte(len(payload) + 2)
	copy(pkt[4:], payload)

	var crc byte
	for _, b := range pkt[2 : 2+int(pkt[3])] {
		crc += b
	}
	pkt[4+len(payload)] = crc

	return pkt
}

func (p *protocolV2) write(cmd byte, payload []byte) error {
	pkt := Packet(cmd, payload)

	log.Debug().Uint8("cmd", cmd).Hex("pkt", pkt[:4+len(payload)+1]).Msg("sending v2 command")

	buf := make([]byte, 1+reportSize)
	buf[0] = p.outReportID
	copy(buf[1:], pkt)

	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	_, err := p.dev.Write(buf)
	return err
}
