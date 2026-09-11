package commands

import (
	"errors"
	"fmt"

	"github.com/pipe01/flydigictl/pkg/dbus/client"
	"github.com/spf13/cobra"
)

func asFlydigiError(err error, target *client.FlydigiError) bool {
	return errors.As(err, target)
}

var takeoverCommand = &cobra.Command{
	Use:   "takeover [on|off]",
	Short: "Get or set \"Allow third-party apps to take over mappings\"",
	Long: `Reads or changes the "Allow third-party apps to take over mappings" setting.

When enabled, Steam (SDL), reWASD and similar apps can acquire the controller
and see every button, including the extra back/side buttons. When disabled the
controller only exposes a plain XInput gamepad.

Only supported by controllers using Flydigi's new protocol (Vader 5 Pro, Apex 5,
Apex 6). Vader 4 Pro and older controllers don't have this setting.`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"on", "off"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return useConnection(func() error {
			if len(args) == 0 {
				status, err := dbusClient.GetTakeover()
				if err != nil {
					return err
				}

				if terseOutput {
					if status.Enabled {
						fmt.Println("on")
					} else {
						fmt.Println("off")
					}
				} else {
					fmt.Printf("Third-party takeover: %s\n", takeoverString(status))
				}

				return nil
			}

			var enable bool
			switch args[0] {
			case "on", "enable", "enabled", "true", "1":
				enable = true
			case "off", "disable", "disabled", "false", "0":
				enable = false
			default:
				return fmt.Errorf("invalid value %q, expected on or off", args[0])
			}

			status, err := dbusClient.SetTakeover(enable)
			if err != nil {
				return err
			}

			if !terseOutput {
				fmt.Printf("Third-party takeover: %s\n", takeoverString(status))
			}

			return nil
		})
	},
}

var takeoverAutoCommand = &cobra.Command{
	Use:   "auto [on|off]",
	Short: "Re-apply the takeover setting automatically whenever the controller is plugged in",
	Long: `Some controllers lose the "Allow third-party apps to take over mappings"
setting (for example after a firmware update or a factory reset). With auto mode
enabled, a udev rule re-enables the setting every time a controller using the new
Flydigi protocol is connected. The flag is stored in /etc/flydigictl/auto-takeover.

Does not need the controller to be connected.`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"on", "off"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			enabled, err := dbusClient.GetAutoTakeover()
			if err != nil {
				return err
			}

			if terseOutput {
				if enabled {
					fmt.Println("on")
				} else {
					fmt.Println("off")
				}
			} else if enabled {
				fmt.Println("Auto re-apply: enabled")
			} else {
				fmt.Println("Auto re-apply: disabled")
			}

			return nil
		}

		var enable bool
		switch args[0] {
		case "on", "enable", "enabled", "true", "1":
			enable = true
		case "off", "disable", "disabled", "false", "0":
			enable = false
		default:
			return fmt.Errorf("invalid value %q, expected on or off", args[0])
		}

		if err := dbusClient.SetAutoTakeover(enable); err != nil {
			return err
		}

		if !terseOutput {
			if enable {
				fmt.Println("Auto re-apply: enabled (takeover will be turned on whenever a controller is plugged in)")
			} else {
				fmt.Println("Auto re-apply: disabled")
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(takeoverCommand)
	takeoverCommand.AddCommand(takeoverAutoCommand)
}
