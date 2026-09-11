package flydigi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pipe01/flydigictl/pkg/flydigi/config"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/dinput"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/v2"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/xinput"
	"github.com/pipe01/flydigictl/pkg/utils"

	"github.com/rs/zerolog/log"
)

type FDGConncetType int32

const (
	FDGConncetUnknow FDGConncetType = iota
	FDGConncetWireless
	FDGConncetWired
)

type FDGDeviceInfo struct {
	DeviceId   int32
	DeviceCode string // Flydigi model code, e.g. "f5"
	DeviceName string // human readable model name

	ProtocolVersion protocol.Version

	FirmwareVersion     string
	FirmwareVersionCode int32
	DongleVersion       string
	SIVersion           string
	TriggerVersion      string
	ScreenVersion       string
	ADCVersion          string
	RFVersion           string

	DeviceMac        string
	MotionSensorType string
	ConnectType      FDGConncetType
	CpuType          string
	CpuName          string

	BatteryPercent int32
	BatteryState   protocol.BatteryState

	GameHadleName string
	FirmwareName  string
}

// TakeoverStatus describes the "Allow third-party apps to take over mappings" setting.
type TakeoverStatus struct {
	Enabled   bool
	ControlBy string
}

type Gamepad struct {
	prot protocol.Protocol

	devInfo *utils.CondValue[FDGDeviceInfo]

	closech chan struct{}

	currConfig    *utils.CondValue[config.AllConfigBean]
	currLEDConfig *utils.CondValue[config.NewLedConfigBean]
	takeover      *utils.CondValue[TakeoverStatus]
	lastAck       *utils.CondValue[protocol.MessageAck]

	configID byte
}

func OpenGamepad() (*Gamepad, error) {
	var prot protocol.Protocol

	openers := []struct {
		name string
		open func() (protocol.Protocol, error)
	}{
		{"v2", v2.Open},
		{"dinput", dinput.Open},
		{"xinput", xinput.Open},
	}

	for _, o := range openers {
		p, err := o.open()
		if err == nil {
			log.Debug().Str("protocol", o.name).Msg("opened gamepad")
			prot = p
			break
		}
		if !errors.Is(err, protocol.ErrGamepadNotPresent) {
			return nil, fmt.Errorf("open %s device: %w", o.name, err)
		}
	}

	if prot == nil {
		return nil, protocol.ErrGamepadNotPresent
	}

	gamepad := &Gamepad{
		prot:          prot,
		closech:       make(chan struct{}),
		devInfo:       utils.NewCondValue[FDGDeviceInfo](&sync.Mutex{}),
		currConfig:    utils.NewCondValue[config.AllConfigBean](&sync.Mutex{}),
		currLEDConfig: utils.NewCondValue[config.NewLedConfigBean](&sync.Mutex{}),
		takeover:      utils.NewCondValue[TakeoverStatus](&sync.Mutex{}),
		lastAck:       utils.NewCondValue[protocol.MessageAck](&sync.Mutex{}),
	}
	go gamepad.readLoop()

	return gamepad, nil
}

func (g *Gamepad) Close() error {
	return g.prot.Close()
}

// ProtocolVersion reports the protocol family spoken by the connected gamepad.
func (g *Gamepad) ProtocolVersion() protocol.Version {
	return g.prot.Version()
}

func (g *Gamepad) readLoop() {
	defer close(g.closech)
	defer g.prot.Close()

	for msg := range g.prot.Messages() {
		if err := g.handleMessage(msg); err != nil {
			log.Err(err).Msg("failed to handle usb data")
		}
	}

	log.Debug().Msg("gamepad read loop exited")
}

func (g *Gamepad) NotifyClose(ch chan<- struct{}) {
	go func() {
		<-g.closech
		ch <- struct{}{}
	}()
}

func (g *Gamepad) handleMessage(msg protocol.Message) error {
	switch msg := msg.(type) {
	case protocol.MessageGamePadInfo:
		return g.handleDeviceInfo(msg)

	case protocol.MessageDongleInfo:
		return g.handleDongleInfo(msg)

	case protocol.MessageGamepadConfigReadCB:
		return g.handleGamepadConfigRead(msg)

	case protocol.MessageLEDConfigReadCB:
		return g.handleLEDConfigRead(msg)

	case protocol.MessageTakeoverStatus:
		g.takeover.Value = &TakeoverStatus{Enabled: msg.Enabled, ControlBy: msg.ControlBy}
		g.takeover.Broadcast()
		return nil

	case protocol.MessageAck:
		g.lastAck.Value = &msg
		g.lastAck.Broadcast()
		return nil

	default:
		return errors.New("unknown message type")
	}
}

func (g *Gamepad) handleDeviceInfo(msg protocol.MessageGamePadInfo) error {
	log.Debug().Uint8("deviceid", msg.DeviceID).Msg("got device info")

	devInfo := FDGDeviceInfo{}

	devInfo.DeviceId = int32(msg.DeviceID)
	devInfo.ProtocolVersion = g.prot.Version()

	dev := config.LookupDevice(devInfo.DeviceId)
	devInfo.DeviceCode = dev.Code
	devInfo.DeviceName = dev.Name

	devInfo.DeviceMac = net.HardwareAddr(msg.DeviceMac).String()

	fw_l := msg.FW_L & 15
	fw_l_2 := msg.FW_L >> 4
	fw_h := msg.FW_H & 15
	fw_h_2 := msg.FW_H >> 4

	devInfo.FirmwareVersionCode = int32(fw_h_2)*1000 + int32(fw_h)*100 + int32(fw_l_2)*10 + int32(fw_l)
	devInfo.FirmwareVersion = fmt.Sprintf("%d.%d.%d.%d", fw_h_2, fw_h, fw_l_2, fw_l)

	switch msg.MotionSensorType {
	case 1:
		devInfo.MotionSensorType = "ST"
	case 2:
		devInfo.MotionSensorType = "QST"
	}

	if msg.Extended != nil {
		g.fillExtendedInfo(&devInfo, msg)
	} else {
		g.fillLegacyInfo(&devInfo, msg, fw_h_2, fw_h)
	}

	devInfo.GameHadleName = config.GameHandleName[devInfo.DeviceId]
	if devInfo.GameHadleName == "" {
		devInfo.GameHadleName = devInfo.DeviceCode
	}

	switch devInfo.DeviceId {
	case 19:
		devInfo.FirmwareName = "apex2"

	case 20, 21:
		devInfo.FirmwareName = "f1"
		if devInfo.CpuType == "wch" {
			devInfo.FirmwareName = "f1wch"
		}

	case 22, 23:
		devInfo.FirmwareName = "f1p"

	case 24:
		devInfo.FirmwareName = "k1"

	case 25:
		devInfo.FirmwareName = "fp1"

	default:
		devInfo.FirmwareName = devInfo.DeviceCode
	}

	g.devInfo.Value = &devInfo
	g.devInfo.Broadcast()

	return nil
}

// fillExtendedInfo handles device info from V2 controllers.
func (g *Gamepad) fillExtendedInfo(devInfo *FDGDeviceInfo, msg protocol.MessageGamePadInfo) {
	ext := msg.Extended

	devInfo.FirmwareVersion = ext.FirmwareVersion
	devInfo.DongleVersion = ext.DongleVersion
	devInfo.SIVersion = ext.SIVersion
	devInfo.TriggerVersion = ext.TriggerVersion
	devInfo.ScreenVersion = ext.ScreenVersion
	devInfo.ADCVersion = ext.ADCVersion
	devInfo.RFVersion = ext.RFVersion

	devInfo.BatteryState = ext.BatteryState
	devInfo.BatteryPercent = int32(ext.BatteryLevel * 20)
	if devInfo.BatteryPercent > 100 {
		devInfo.BatteryPercent = 100
	}

	devInfo.CpuType = fmt.Sprintf("type %d", ext.ChipType)
	devInfo.CpuName = ""

	switch msg.ConnectionType {
	case 1:
		devInfo.ConnectType = FDGConncetWired
	case 2:
		devInfo.ConnectType = FDGConncetWireless
	}
}

// fillLegacyInfo handles device info from V1 controllers.
func (g *Gamepad) fillLegacyInfo(devInfo *FDGDeviceInfo, msg protocol.MessageGamePadInfo, fw_h_2, fw_h byte) {
	battery := msg.Battery
	const apex2MinBY = 98
	const apex2MaxBY = 114

	if battery < apex2MinBY {
		battery = apex2MinBY
	} else if battery > apex2MaxBY {
		battery = apex2MaxBY
	}

	batteryPercent := int(100 * float32(battery-apex2MinBY) / float32(apex2MaxBY-apex2MinBY))
	devInfo.BatteryPercent = int32(batteryPercent)

	if msg.CPUType > 0 {
		devInfo.CpuType = "wch"
	} else {
		devInfo.CpuType = "nordic"
	}

	if fw_h_2 >= 6 && fw_h >= 1 {
		devInfo.CpuType = "wch"
	}

	if devInfo.CpuType == "wch" {
		if msg.ConnectionType == 1 {
			devInfo.ConnectType = FDGConncetWired
			devInfo.CpuName = "ch573"
		} else {
			devInfo.ConnectType = FDGConncetWireless
			devInfo.CpuName = "ch571"

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			g.prot.Send(ctx, protocol.CommandGetDongleVersion{})
		}
	}
}

func (g *Gamepad) handleDongleInfo(msg protocol.MessageDongleInfo) error {
	fw_l := msg.FW_L & 15
	fw_l_2 := msg.FW_L >> 4
	fw_h := msg.FW_H & 15
	fw_h_2 := msg.FW_H >> 4

	if g.devInfo.Value == nil {
		g.devInfo.Value = &FDGDeviceInfo{}
	}

	if fw_l+fw_l_2+fw_h+fw_h_2 > 0 {
		g.devInfo.Value.DongleVersion = fmt.Sprintf("%d.%d.%d.%d", fw_l_2, fw_l, fw_h_2, fw_h)
		g.devInfo.Value.ConnectType = FDGConncetWireless
	} else {
		g.devInfo.Value.ConnectType = FDGConncetWired
	}

	return nil
}

func (g *Gamepad) handleGamepadConfigRead(msg protocol.MessageGamepadConfigReadCB) error {
	log.Debug().Int("length", len(msg.Data)).Msg("got gamepad configuration data")

	cfg, err := config.ConvertGPConfigByByte(msg.Data)
	if err != nil {
		return fmt.Errorf("convert GP config: %w", err)
	}

	g.currConfig.Value = cfg
	g.currConfig.Broadcast()

	return nil
}

func (g *Gamepad) handleLEDConfigRead(msg protocol.MessageLEDConfigReadCB) error {
	log.Debug().Int("length", len(msg.Data)).Msg("got led configuration data")

	cfg := config.ConvertLEDConfigByByte(msg.Data)

	if g.currConfig.Value != nil {
		g.currConfig.Value.Basic.NewLedConfig = cfg
	}

	g.currLEDConfig.Value = cfg
	g.currLEDConfig.Broadcast()

	return nil
}

func (g *Gamepad) SaveConfig(ctx context.Context, cfg *config.AllConfigBean) error {
	var buf bytes.Buffer
	config.ConvertByteByGConfig(&buf, cfg)

	log.Info().Int("length", buf.Len()).Msg("saving gamepad configuration")

	if err := g.prot.Send(ctx, protocol.CommandSendConfig{
		Data:     buf.Bytes(),
		ConfigID: g.configID,
	}); err != nil {
		return fmt.Errorf("send config: %w", err)
	}

	g.currConfig.Value = nil

	buf.Reset()

	if cfg.Basic.NewLedConfig != nil {
		if err := g.SaveLEDConfig(ctx, cfg.Basic.NewLedConfig); err != nil {
			return fmt.Errorf("save led config: %w", err)
		}
	}

	return nil
}

func (g *Gamepad) SaveLEDConfig(ctx context.Context, cfg *config.NewLedConfigBean) error {
	var buf bytes.Buffer
	config.ConvertByteByNewLedConfig(&buf, cfg)

	log.Info().Int("length", buf.Len()).Msg("saving led configuration")

	if err := g.prot.Send(ctx, protocol.CommandSendLEDConfig{
		Data:     buf.Bytes(),
		ConfigID: g.configID,
	}); err != nil {
		return fmt.Errorf("send config: %w", err)
	}

	g.currLEDConfig.Value = nil

	return nil
}

var errNoResponse = errors.New("device doesn't respond")

func getConfigRetry[T any](ctx context.Context, prot protocol.Protocol, v *utils.CondValue[T], cmd protocol.Command) (*T, error) {
	if v.Value == nil {
		retriesLeft := 3

		for retriesLeft > 0 {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			notify := v.NotifyChan()

			err := prot.Send(ctx, cmd)
			if err != nil {
				return nil, fmt.Errorf("send command: %w", err)
			}

			select {
			case <-notify:
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				retriesLeft--
				continue
			}

			return v.Value, nil
		}

		return nil, errNoResponse
	}

	return v.Value, nil
}

func (g *Gamepad) GetConfig(ctx context.Context) (*config.AllConfigBean, error) {
	return getConfigRetry(ctx, g.prot, g.currConfig, protocol.CommandReadConfig{ConfigID: g.configID})
}

func (g *Gamepad) GetLEDConfig(ctx context.Context) (*config.NewLedConfigBean, error) {
	return getConfigRetry(ctx, g.prot, g.currLEDConfig, protocol.CommandReadLEDConfig{ConfigID: g.configID})
}

func (g *Gamepad) GetGamepadInfo(ctx context.Context) (*FDGDeviceInfo, error) {
	return getConfigRetry(ctx, g.prot, g.devInfo, protocol.CommandGetDeviceInfo{})
}

// GetTakeover reads the third-party takeover setting. Always queries the device.
func (g *Gamepad) GetTakeover(ctx context.Context) (*TakeoverStatus, error) {
	g.takeover.Value = nil
	return getConfigRetry(ctx, g.prot, g.takeover, protocol.CommandGetTakeover{})
}

// waitAck sends a command and waits for an acknowledgement with the given command id.
func (g *Gamepad) waitAck(ctx context.Context, cmd protocol.Command, ackCmd byte, timeout time.Duration) (*protocol.MessageAck, error) {
	notify := g.lastAck.NotifyChan()

	if err := g.prot.Send(ctx, cmd); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	deadline := time.After(timeout)

	for {
		select {
		case <-notify:
			ack := g.lastAck.Value
			notify = g.lastAck.NotifyChan()
			if ack != nil && ack.Command == ackCmd {
				return ack, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, errNoResponse
		}
	}
}

// SetTakeover enables or disables the third-party takeover setting and verifies the result.
func (g *Gamepad) SetTakeover(ctx context.Context, enable bool) (*TakeoverStatus, error) {
	if _, err := g.waitAck(ctx, protocol.CommandSetTakeover{Enable: enable}, v2.CmdEnableRawData, 2*time.Second); err != nil {
		return nil, err
	}

	status, err := g.GetTakeover(ctx)
	if err != nil {
		return nil, fmt.Errorf("read back status: %w", err)
	}

	if status.Enabled != enable {
		return status, errors.New("device did not apply the setting")
	}

	return status, nil
}

// Calibrate starts or finishes the joystick/trigger calibration procedure.
func (g *Gamepad) Calibrate(ctx context.Context, start bool) error {
	cmd := protocol.CommandCalibrate{Start: start}

	if g.prot.Version() == protocol.VersionV2 {
		_, err := g.waitAck(ctx, cmd, v2.CmdCalibrationADC, 2*time.Second)
		return err
	}

	// V1 devices don't reliably acknowledge; fire and forget like Flydigi Space does.
	return g.prot.Send(ctx, cmd)
}
