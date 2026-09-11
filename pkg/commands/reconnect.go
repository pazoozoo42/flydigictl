package commands

import (
	"fmt"
	"time"

	"github.com/pipe01/flydigictl/pkg/dbus"
	"github.com/pipe01/flydigictl/pkg/dbus/client"
	"github.com/pipe01/flydigictl/pkg/dbus/pb"
	"github.com/spf13/cobra"
)

var reconnectCommand = &cobra.Command{
	Use:   "reconnect",
	Short: "Re-plug the controller in software so Steam re-detects it",
	Long: `Makes the kernel drop and re-enumerate the controller's USB device, which looks
exactly like unplugging the cable and plugging it back in to Steam and other
applications, without power-cycling the controller.

Use this when Steam only sees the controller as a plain "Generic X-Box pad"
although third-party takeover is enabled: the controller is often not ready to
answer yet in the first moments after it is plugged in, so Steam's Flydigi
driver gives up and falls back to XInput. See also "flydigictl hotplug", which
does this automatically.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := reconnectGamepad(); err != nil {
			return err
		}

		if !terseOutput {
			fmt.Println("Controller re-enumerated, waiting for it to come back...")
		}

		status, err := waitTakeoverStatus(6*time.Second, func(st *pb.TakeoverStatus) bool { return st.ControlBy != "" })
		if err != nil {
			return err
		}

		if terseOutput {
			fmt.Println(takeoverString(status))
		} else {
			fmt.Printf("Third-party takeover: %s\n", takeoverString(status))
		}
		return nil
	},
}

// reconnectGamepad asks the daemon to re-enumerate the controller's USB device. The daemon
// drops its gamepad connection as part of this, so no Disconnect must follow.
func reconnectGamepad() error {
	if err := connectGamepad(); err != nil {
		return fmt.Errorf("connect to gamepad: %w", err)
	}

	if err := dbusClient.Reconnect(); err != nil {
		disconnectGamepad()
		return err
	}

	weConnectedGamepad = false
	return nil
}

// waitTakeoverStatus polls the controller until it answers and `done` accepts the status,
// or the timeout expires. The last status read is returned in either case; an error is
// returned only if the controller never answered.
func waitTakeoverStatus(timeout time.Duration, done func(*pb.TakeoverStatus) bool) (*pb.TakeoverStatus, error) {
	deadline := time.Now().Add(timeout)

	var (
		last    *pb.TakeoverStatus
		lastErr error
	)

	for {
		status, err := readTakeoverStatus()
		if err == nil {
			last = status
			if done == nil || done(status) {
				return status, nil
			}
		} else {
			var ferr client.FlydigiError
			if asFlydigiError(err, &ferr) && ferr.Name == dbus.ErrorUnsupported {
				return nil, err
			}
			lastErr = err
		}

		if time.Now().After(deadline) {
			if last != nil {
				return last, nil
			}
			return nil, fmt.Errorf("controller did not answer: %w", lastErr)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func readTakeoverStatus() (*pb.TakeoverStatus, error) {
	if err := connectGamepad(); err != nil {
		return nil, err
	}
	defer disconnectGamepad()

	if _, err := dbusClient.GetDeviceInfo(); err != nil {
		return nil, err
	}

	return dbusClient.GetTakeover()
}

func init() {
	rootCmd.AddCommand(reconnectCommand)
}
