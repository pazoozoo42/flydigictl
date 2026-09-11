// Package xinput implements the legacy (V1) Flydigi protocol over the vendor
// specific Xbox 360 interface exposed by controllers in XInput mode
// (VID 045e, PID 028e, interface 0, endpoints 0x81 IN / 0x05 OUT).
package xinput

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/pipe01/flydigictl/pkg/flydigi/protocol"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/internal"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/usbfs"

	"github.com/rs/zerolog/log"
)

const (
	VendorID  = 0x045e
	ProductID = 0x028e
	Interface = 0

	endpointIn  = 0x81
	endpointOut = 0x05
)

const (
	packageLength    = 52
	ledPackageLength = 49
)

const (
	commandGetDongleVersion = 17
	commandReadConfig       = 33
	commandGetDeviceInfo    = 16
	commandReadLEDConfig    = 38
	commandCalibration      = 20
)

type protocolXInput struct {
	dev  *usbfs.Device
	info usbfs.DeviceInfo

	isClosed atomic.Bool

	msgch chan protocol.Message

	configReader, ledConfigReader *internal.ConfigReader

	configWriter *internal.ConfigWriter
}

// outWriter adapts the OUT endpoint to io.Writer for the config writer.
type outWriter struct {
	p *protocolXInput
}

func (w outWriter) Write(p []byte) (int, error) {
	return w.p.dev.Transfer(endpointOut, p, 1000)
}

func Open() (protocol.Protocol, error) {
	devs, err := usbfs.Find(VendorID, ProductID)
	if err != nil {
		return nil, fmt.Errorf("enumerate devices: %w", err)
	}

	log.Debug().Int("count", len(devs)).Msg("found xinput usb devices")

	if len(devs) == 0 {
		return nil, protocol.ErrGamepadNotPresent
	}

	dev, err := usbfs.Open(devs[0].Path, Interface)
	if err != nil {
		return nil, fmt.Errorf("open usb device: %w", err)
	}

	p := &protocolXInput{
		dev:             dev,
		info:            devs[0],
		msgch:           make(chan protocol.Message, 10),
		configReader:    internal.NewConfigReader(packageLength, 10),
		ledConfigReader: internal.NewConfigReader(ledPackageLength, 10),
	}
	p.configWriter = internal.NewConfigWriter(outWriter{p})

	go p.readLoop()

	return p, nil
}

func (d *protocolXInput) USBDevicePath() string {
	return d.info.SysPath
}

func (d *protocolXInput) Close() error {
	if d.isClosed.Swap(true) {
		return nil
	}

	return d.dev.Close()
}

func (d *protocolXInput) Version() protocol.Version {
	return protocol.VersionV1
}

func (d *protocolXInput) Messages() <-chan protocol.Message {
	return d.msgch
}

func (d *protocolXInput) readLoop() {
	buf := make([]byte, 100)

	defer close(d.msgch)

	for !d.isClosed.Load() {
		n, err := d.dev.Transfer(endpointIn, buf, 500)
		if err != nil {
			if usbfs.IsTimeout(err) {
				continue
			}
			if !d.isClosed.Load() && !usbfs.IsNoDevice(err) {
				log.Err(err).Msg("failed to read data from usb")
			}
			break
		}

		if n < 32 {
			continue
		}

		msg, ok := d.resolveUsbData(buf[:n])
		if ok {
			d.msgch <- msg
		}
	}
}

func (d *protocolXInput) resolveUsbData(p []byte) (protocol.Message, bool) {
	if p[14] == 0xA5 {
		switch p[15] {
		case 16:
			return protocol.MessageGamePadInfo{
				DeviceID:         p[16],
				DeviceMac:        p[17:21],
				FW_L:             p[21],
				FW_H:             p[22],
				Battery:          p[23],
				CPUType:          p[24],
				ConnectionType:   p[25],
				MotionSensorType: p[26],
			}, true

		case 17:
			return protocol.MessageDongleInfo{
				FW_L: p[16],
				FW_H: p[17],
			}, true

		case commandCalibration:
			return protocol.MessageAck{Command: commandCalibration, Data: append([]byte(nil), p...)}, true

		case 32:
			// HandleGamepadConfigId

		case 34:
			// HandleGamepadConfigReadCB
			d.configReader.GotPackage(int(p[16]), p[17:28])

			if d.configReader.IsFinished() {
				time.Sleep(200 * time.Millisecond)
				return protocol.MessageGamepadConfigReadCB{
					Data: d.configReader.Data(),
				}, true
			}

		case 35, 37: // HandleStartWriteGamepadConfig
			d.configWriter.Ack(0)

		case 36: // HandleWriteGamepadConfigCBK
			d.configWriter.Ack(int(p[16]))

		case 39:
			// HandleLedConfigReadCB
			d.ledConfigReader.GotPackage(int(p[16]), p[17:28])

			if d.ledConfigReader.IsFinished() {
				time.Sleep(200 * time.Millisecond)
				return protocol.MessageLEDConfigReadCB{
					Data: d.ledConfigReader.Data(),
				}, true
			}

		case 41: // HandleWriteLEDConfigCBK
			d.configWriter.Ack(int(p[16]))

		case 42: // HandleStartWriteLEDConfig
			d.configWriter.Ack(0)
		}
	}

	return nil, false
}

func (d *protocolXInput) Send(ctx context.Context, cmd protocol.Command) error {
	switch cmd := cmd.(type) {
	case protocol.CommandGetDongleVersion:
		return d.sendCommand(ctx, commandGetDongleVersion)

	case protocol.CommandGetDeviceInfo:
		return d.sendCommand(ctx, commandGetDeviceInfo)

	case protocol.CommandReadConfig:
		d.configReader.Reset()
		return d.sendCommand(ctx, commandReadConfig, cmd.ConfigID)

	case protocol.CommandReadLEDConfig:
		d.ledConfigReader.Reset()
		return d.sendCommand(ctx, commandReadLEDConfig, cmd.ConfigID)

	case protocol.CommandSendConfig:
		return d.sendConfig(ctx, cmd.Data, cmd.ConfigID, false)

	case protocol.CommandSendLEDConfig:
		return d.sendConfig(ctx, cmd.Data, cmd.ConfigID, true)

	case protocol.CommandCalibrate:
		// Flydigi Space: { A5, 20, status, ... } with status 1 = start, 2 = finish
		status := byte(2)
		if cmd.Start {
			status = 1
		}
		return d.sendCommand(ctx, commandCalibration, status)

	case protocol.CommandGetTakeover, protocol.CommandSetTakeover:
		return protocol.ErrUnsupported

	default:
		return protocol.ErrUnknownCommand
	}
}

func (d *protocolXInput) sendCommand(ctx context.Context, cmd byte, args ...byte) error {
	log.Debug().Uint8("cmd", cmd).Bytes("args", args).Msg("sending command")

	pkg := make([]byte, 15)
	pkg[0] = 165
	pkg[1] = cmd
	copy(pkg[2:], args)

	_, err := d.dev.Transfer(endpointOut, crcData(pkg), 1000)
	return err
}

func (g *protocolXInput) sendConfig(ctx context.Context, data []byte, configID byte, isLED bool) error {
	var chunks [][]byte
	if isLED {
		chunks = getLEDConfigDataParcels(data, configID)
	} else {
		chunks = getConfigDataParcels(data, configID)
	}

	return g.configWriter.Send(ctx, chunks, 3, 3*time.Second)
}
