package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pipe01/flydigictl/pkg/dbus/pb"
	"github.com/pipe01/flydigictl/pkg/firmware"
	"github.com/spf13/cobra"
)

var (
	firmwareJSON       bool
	firmwareLatest     bool
	firmwareAPIBase    string
	firmwareAppVersion string
)

var firmwareCommand = &cobra.Command{
	Use:   "firmware",
	Short: "Firmware related commands",
}

var firmwareCheckCommand = &cobra.Command{
	Use:   "check",
	Short: "Check Flydigi's update service for newer firmware",
	Long: `Asks Flydigi's firmware update service (the same one Flydigi Space Station
uses) whether a newer firmware exists for the connected controller.

When connected through the wireless dongle the dongle firmware is checked,
otherwise the controller's main firmware is checked. Flashing is not
supported; use Flydigi Space Station on Windows to install an update.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var info *pb.GamepadInfo

		err := useConnection(func() error {
			var err error
			info, err = dbusClient.GetDeviceInfo()
			return err
		})
		if err != nil {
			return fmt.Errorf("get device info: %w", err)
		}

		if info.DeviceCode == "" {
			return fmt.Errorf("unknown device id %d, cannot query firmware", info.DeviceId)
		}

		wireless := info.ConnectionType == pb.ConnectionType_WIRELESS

		req := firmware.Request{
			DeviceCode:    info.DeviceCode,
			DeviceID:      info.DeviceId,
			Wireless:      wireless,
			APIBase:       firmwareAPIBase,
			AppVersion:    firmwareAppVersion,
			MainVersion:   info.FirmwareVersion,
			DongleVersion: info.DongleVersion,
		}
		if firmwareLatest {
			req.MainVersion = ""
			req.DongleVersion = ""
		}

		resp, err := firmware.Check(cmd.Context(), req)
		if err != nil {
			return err
		}

		chipKey, current := "main_chip", info.FirmwareVersion
		if wireless {
			chipKey, current = "dongle_chip", info.DongleVersion
		}

		chip := resp.Chips[chipKey]

		if firmwareJSON {
			out := map[string]any{
				"device_id":       info.DeviceId,
				"device_code":     info.DeviceCode,
				"device_name":     info.DeviceName,
				"chip":            chipKey,
				"current_version": current,
				"update":          chip,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		name := info.DeviceName
		if name == "" {
			name = "Unknown"
		}

		fmt.Printf("Device: %s (%s, id %d)\n", name, info.DeviceCode, info.DeviceId)
		fmt.Printf("Checked chip: %s\n", chipKey)
		fmt.Printf("Current version: %s\n", current)

		if chip == nil || chip.Version == "" {
			if firmwareLatest {
				fmt.Println("The update service has no firmware listed for this device.")
			} else {
				fmt.Println("Firmware is up to date.")
			}
			return nil
		}

		switch firmware.CompareVersions(chip.Version, current) {
		case 1:
			fmt.Printf("Update available: %s\n", chip.Version)
		case 0:
			fmt.Printf("Latest version: %s (installed)\n", chip.Version)
		default:
			fmt.Printf("Latest published version: %s (older than installed)\n", chip.Version)
		}

		if chip.Info != "" {
			fmt.Printf("Notes: %s\n", chip.Info)
		}
		if chip.URL != "" {
			fmt.Printf("Download: %s\n", chip.URL)
		}
		if chip.IsPush != 0 {
			fmt.Println("Flydigi marks this update as recommended.")
		}
		fmt.Println("Install it with Flydigi Space Station (Windows) over a USB cable.")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(firmwareCommand)
	firmwareCommand.AddCommand(firmwareCheckCommand)

	firmwareCheckCommand.Flags().BoolVar(&firmwareJSON, "json", false, "output as JSON")
	firmwareCheckCommand.Flags().BoolVar(&firmwareLatest, "latest", false, "ask for the latest version regardless of the installed one")
	firmwareCheckCommand.Flags().StringVar(&firmwareAPIBase, "api", firmware.DefaultAPIBase, "update service base URL")
	firmwareCheckCommand.Flags().StringVar(&firmwareAppVersion, "app-version", firmware.DefaultAppVersion, "Flydigi Space Station version to report")
}
