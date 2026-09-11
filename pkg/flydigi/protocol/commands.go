package protocol

type cmd struct{}

func (cmd) command() {}

type CommandGetDongleVersion struct {
	cmd
}

type CommandGetDeviceInfo struct {
	cmd
}

type CommandReadConfig struct {
	cmd
	ConfigID byte
}

type CommandReadLEDConfig struct {
	cmd
	ConfigID byte
}

type CommandSendConfig struct {
	cmd
	Data     []byte
	ConfigID byte
}

type CommandSendLEDConfig struct {
	cmd
	Data     []byte
	ConfigID byte
}

// CommandGetTakeover queries whether third-party apps (Steam, reWASD...) are
// allowed to take over the controller mappings. V2 only.
type CommandGetTakeover struct {
	cmd
}

// CommandSetTakeover enables or disables third-party takeover. V2 only.
type CommandSetTakeover struct {
	cmd
	Enable bool
}

// CommandCalibrate starts (Start=true) or finishes (Start=false) the
// joystick/trigger ADC calibration procedure.
type CommandCalibrate struct {
	cmd
	Start bool
}
