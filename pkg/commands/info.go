package commands

import (
	"fmt"
	"strings"

	"github.com/pipe01/flydigictl/pkg/dbus"
	"github.com/pipe01/flydigictl/pkg/dbus/client"
	"github.com/pipe01/flydigictl/pkg/dbus/pb"
	"github.com/spf13/cobra"
)

func batteryStateString(s pb.BatteryState) string {
	switch s {
	case pb.BatteryState_ON_BATTERY:
		return "on battery"
	case pb.BatteryState_CHARGING:
		return "charging"
	case pb.BatteryState_CHARGED:
		return "charged"
	}
	return ""
}

var infoCommand = &cobra.Command{
	Use:   "info",
	Short: "Get information about the connected gamepad",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return useConnection(func() error {
			info, err := dbusClient.GetDeviceInfo()
			if err != nil {
				return fmt.Errorf("get device info: %w", err)
			}

			type entry struct {
				Key   string
				Value any
			}

			name := info.DeviceName
			if name == "" {
				name = "Unknown"
			}

			battery := fmt.Sprintf("%d%%", info.BatteryPercent)
			if s := batteryStateString(info.BatteryState); s != "" {
				battery += " (" + s + ")"
			}

			dict := []entry{
				{"Device", fmt.Sprintf("%d (%s)", info.DeviceId, name)},
				{"Model code", info.DeviceCode},
				{"Protocol", fmt.Sprintf("v%d", info.ProtocolVersion)},
				{"Firmware", info.FirmwareVersion},
			}

			optional := []entry{
				{"Dongle firmware", info.DongleVersion},
				{"SI firmware", info.SiVersion},
				{"RF firmware", info.RfVersion},
				{"Trigger firmware", info.TriggerVersion},
				{"Screen firmware", info.ScreenVersion},
				{"ADC firmware", info.AdcVersion},
			}
			for _, e := range optional {
				if e.Value.(string) != "" {
					dict = append(dict, e)
				}
			}

			dict = append(dict,
				entry{"Battery", battery},
				entry{"Connection type", strings.ToLower(info.ConnectionType.String())},
			)

			if info.ProtocolVersion == 1 {
				dict = append(dict, entry{"CPU", fmt.Sprintf("%s (%s)", info.CpuType, info.CpuName)})
			}

			if info.ProtocolVersion == 2 {
				status, err := dbusClient.GetTakeover()
				if err != nil {
					var ferr client.FlydigiError
					if !(asFlydigiError(err, &ferr) && ferr.Name == dbus.ErrorUnsupported) {
						return fmt.Errorf("get takeover status: %w", err)
					}
				} else {
					dict = append(dict, entry{"Third-party takeover", takeoverString(status)})
				}
			}

			maxLen := 0
			for _, e := range dict {
				if len(e.Key) > maxLen {
					maxLen = len(e.Key)
				}
			}

			for _, e := range dict {
				space := strings.Repeat(" ", maxLen-len(e.Key))

				fmt.Printf("%s%s : %v\n", space, e.Key, e.Value)
			}

			return nil
		})
	},
}

func takeoverString(status *pb.TakeoverStatus) string {
	if status == nil {
		return "unknown"
	}
	if !status.Enabled {
		return "disabled"
	}
	if status.ControlBy != "" {
		return fmt.Sprintf("enabled (held by %q)", status.ControlBy)
	}
	return "enabled"
}

func init() {
	rootCmd.AddCommand(infoCommand)
}
