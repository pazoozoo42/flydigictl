package client

import (
	"fmt"

	"github.com/godbus/dbus/v5"
	common "github.com/pipe01/flydigictl/pkg/dbus"
	"github.com/pipe01/flydigictl/pkg/dbus/pb"
	"google.golang.org/protobuf/proto"
)

type FlydigiError struct {
	Name    string
	Message string
}

func (e FlydigiError) Error() string {
	return e.Message
}

type Client struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

func Dial(useSessionBus bool) (*Client, error) {
	var connector func(opts ...dbus.ConnOption) (*dbus.Conn, error)
	if useSessionBus {
		connector = dbus.ConnectSessionBus
	} else {
		connector = dbus.ConnectSystemBus
	}

	conn, err := connector()
	if err != nil {
		return nil, fmt.Errorf("connect to system bus: %w", err)
	}

	dbusObj := conn.Object(common.InterfaceName, common.ObjectPath)

	return &Client{
		conn: conn,
		obj:  dbusObj,
	}, nil
}

func (c *Client) call(methodName string, args []interface{}, retvalues ...interface{}) error {
	call := c.obj.Call(fmt.Sprintf("%s.%s", common.InterfaceName, methodName), 0, args...)

	return call.Store(retvalues...)
}

func (c *Client) Connect() error {
	return c.wrapError(c.call("Connect", nil))
}

func (c *Client) Disconnect() error {
	return c.wrapError(c.call("Disconnect", nil))
}

func (c *Client) GetServerVersion() (string, error) {
	var version string

	if err := c.call("GetServerVersion", nil, &version); err != nil {
		return "", err
	}

	return version, nil
}

func (c *Client) DumpConfiguration(noColor bool) (string, error) {
	var dump string

	if err := c.call("DumpConfiguration", []any{noColor}, &dump); err != nil {
		return "", err
	}

	return dump, nil
}

func (c *Client) GetConfiguration() (*pb.GamepadConfiguration, error) {
	var cfgBytes []byte

	if err := c.call("GetConfiguration", nil, &cfgBytes); err != nil {
		return nil, c.wrapError(err)
	}

	var cfg pb.GamepadConfiguration

	if err := proto.Unmarshal(cfgBytes, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

func (c *Client) SetConfiguration(cfg *pb.GamepadConfiguration) error {
	cfgBytes, err := proto.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := c.call("SetConfiguration", []any{cfgBytes}); err != nil {
		return c.wrapError(err)
	}

	return nil
}

func (c *Client) GetLEDConfiguration() (*pb.LedsConfiguration, error) {
	var cfgBytes []byte

	if err := c.call("GetLEDConfiguration", nil, &cfgBytes); err != nil {
		return nil, c.wrapError(err)
	}

	var cfg pb.LedsConfiguration

	if err := proto.Unmarshal(cfgBytes, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

func (c *Client) SetLEDConfiguration(cfg *pb.LedsConfiguration) error {
	cfgBytes, err := proto.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := c.call("SetLEDConfiguration", []any{cfgBytes}); err != nil {
		return c.wrapError(err)
	}

	return nil
}

func (c *Client) GetDeviceInfo() (*pb.GamepadInfo, error) {
	var infoBytes []byte

	if err := c.call("GetDeviceInfo", nil, &infoBytes); err != nil {
		return nil, c.wrapError(err)
	}

	var info pb.GamepadInfo

	if err := proto.Unmarshal(infoBytes, &info); err != nil {
		return nil, fmt.Errorf("unmarshal info: %w", err)
	}

	return &info, nil
}

func (c *Client) GetTakeover() (*pb.TakeoverStatus, error) {
	var b []byte

	if err := c.call("GetTakeover", nil, &b); err != nil {
		return nil, c.wrapError(err)
	}

	var status pb.TakeoverStatus

	if err := proto.Unmarshal(b, &status); err != nil {
		return nil, fmt.Errorf("unmarshal status: %w", err)
	}

	return &status, nil
}

func (c *Client) SetTakeover(enabled bool) (*pb.TakeoverStatus, error) {
	var b []byte

	if err := c.call("SetTakeover", []any{enabled}, &b); err != nil {
		return nil, c.wrapError(err)
	}

	var status pb.TakeoverStatus

	if err := proto.Unmarshal(b, &status); err != nil {
		return nil, fmt.Errorf("unmarshal status: %w", err)
	}

	return &status, nil
}

func (c *Client) GetAutoTakeover() (bool, error) {
	var enabled bool

	if err := c.call("GetAutoTakeover", nil, &enabled); err != nil {
		return false, c.wrapError(err)
	}

	return enabled, nil
}

func (c *Client) SetAutoTakeover(enabled bool) error {
	return c.wrapError(c.call("SetAutoTakeover", []any{enabled}))
}

// Reconnect re-enumerates the controller's USB device (software re-plug). The daemon
// drops the gamepad connection; call Connect again afterwards.
func (c *Client) Reconnect() error {
	return c.wrapError(c.call("Reconnect", nil))
}

func (c *Client) Calibrate(start bool) error {
	return c.wrapError(c.call("Calibrate", []any{start}))
}

func (c *Client) wrapError(err error) error {
	switch err := err.(type) {
	case dbus.Error:
		var msg string

		switch err.Name {
		case common.ErrorNotConnected:
			msg = "gamepad not connected"
		case common.ErrorAlreadyConnected:
			msg = "gamepad already connected"
		case common.ErrorMarshallingFault:
			msg = "marshalling failed"
		case common.ErrorGamepadWritingFault:
			msg = "writing to gamepad failed"
		case common.ErrorGamepadReadingFault:
			msg = "reading from gamepad failed"
		case common.ErrorGamepadNotFound:
			msg = "gamepad not found"
		case common.ErrorUnsupported:
			msg = "not supported by this gamepad or connection mode"
		case common.ErrorReconnectFailed:
			msg = "re-enumerating the usb device failed"
		default:
			return err
		}

		if len(err.Body) > 0 {
			if detail, ok := err.Body[0].(string); ok && detail != "" {
				msg += ": " + detail
			}
		}

		return FlydigiError{Name: err.Name, Message: msg}
	}

	return err
}
