package protocol

type raw []byte

func (raw) message() {}

type MessageGamepadConfigReadCB struct {
	raw
	Data []byte
}

type MessageLEDConfigReadCB struct {
	raw
	Data []byte
}

type MessageDongleInfo struct {
	raw
	FW_L byte
	FW_H byte
}

type MessageWriteGamepadConfigCBK struct {
	raw
	AckNum byte
}

// BatteryState is the charging state reported by V2 controllers.
type BatteryState byte

const (
	BatteryStateUnknown BatteryState = iota
	BatteryStateOnBattery
	BatteryStateCharging
	BatteryStateCharged
)

// ExtendedInfo carries the additional fields reported by V2 controllers.
type ExtendedInfo struct {
	FirmwareVersion string
	DongleVersion   string // empty if not connected through a dongle
	SIVersion       string // SI (Switch/screen interface) chip firmware
	TriggerVersion  string
	ScreenVersion   string
	ADCVersion      string
	RFVersion       string // RF / NearLink chip firmware
	ChipType        byte
	BatteryLevel    int // 0-5
	BatteryState    BatteryState
}

type MessageGamePadInfo struct {
	raw
	DeviceID         byte
	DeviceMac        []byte
	FW_L, FW_H       byte
	Battery          byte
	MotionSensorType byte
	CPUType          byte
	ConnectionType   byte

	// Extended is set by V2 protocols and nil otherwise.
	Extended *ExtendedInfo
}

// MessageTakeoverStatus is the reply to CommandGetTakeover.
type MessageTakeoverStatus struct {
	raw
	XInputEnabled  bool
	RawDataEnabled bool
	Enabled        bool   // third-party takeover allowed
	ControlBy      string // name of the app currently holding the controller (e.g. "SDL")
}

// MessageAck is a generic acknowledgement of a command.
type MessageAck struct {
	raw
	Command byte
	Data    []byte
}
