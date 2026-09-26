package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pipe01/flydigictl/pkg/dbus"
	"github.com/pipe01/flydigictl/pkg/dbus/client"
	"github.com/pipe01/flydigictl/pkg/dbus/pb"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/hidraw"
	"github.com/pipe01/flydigictl/pkg/flydigi/protocol/v2"
	"github.com/spf13/cobra"
)

const (
	hotplugReadyTimeout   = 20 * time.Second
	hotplugAcquireTimeout = 4 * time.Second
	hotplugGuardFile      = "/run/flydigictl/reconnects"
	hotplugGuardInterval  = 60 * time.Second
	hotplugMaxReconnects  = 3
)

var hotplugCommand = &cobra.Command{
	Use:    "hotplug",
	Short:  "Handle a freshly connected controller (run by udev)",
	Hidden: true,
	Long: `Run by flydigictl-hotplug.service whenever a Flydigi controller is connected.

1. Waits until the controller answers.
2. Turns third-party takeover on if "flydigictl takeover auto on" was set.
3. If takeover is enabled and Steam is running but no application has opened
   the controller's HID interface after a few seconds, re-enumerates the USB
   device (see "flydigictl reconnect") so that Steam re-detects it with all
   buttons. At most 3 attempts per minute.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		logf := func(format string, a ...any) { fmt.Fprintf(os.Stderr, "hotplug: "+format+"\n", a...) }

		status, err := waitTakeoverStatus(hotplugReadyTimeout, nil)
		if err != nil {
			var ferr client.FlydigiError
			if asFlydigiError(err, &ferr) && ferr.Name == dbus.ErrorUnsupported {
				logf("controller has no takeover setting, nothing to do")
				return nil
			}
			return err
		}
		logf("controller ready, takeover %s", takeoverString(status))

		auto, err := dbusClient.GetAutoTakeover()
		if err != nil {
			return err
		}

		if auto && !status.Enabled {
			status, err = setTakeover(true)
			if err != nil {
				return fmt.Errorf("enable takeover: %w", err)
			}
			logf("takeover %s (auto)", takeoverString(status))
		}

		if !status.Enabled {
			logf("takeover disabled, nothing to do")
			return nil
		}

		if !steamRunning() {
			logf("steam is not running, nothing to do")
			return nil
		}

		// The controller keeps reporting the previous holder for a while after a
		// re-enumeration, so ask the kernel who actually has the HID interface open.
		holder := waitHIDHolder(hotplugAcquireTimeout)
		status, _ = readTakeoverStatus()
		if holder != "" {
			logf("hid interface opened by %q, controller reports %s; nothing to do", holder, takeoverString(status))
			return nil
		}

		if n := recentReconnects(); n >= hotplugMaxReconnects {
			logf("no application opened the hid interface, but the usb device was already re-enumerated %d times in the last %s; giving up", n, hotplugGuardInterval)
			return nil
		}
		recordReconnect()

		logf("no application opened the hid interface (controller reports %s), re-enumerating the usb device", takeoverString(status))
		if err := reconnectGamepad(); err != nil {
			return err
		}

		return nil
	},
}

func setTakeover(enable bool) (*pb.TakeoverStatus, error) {
	if err := connectGamepad(); err != nil {
		return nil, err
	}
	defer disconnectGamepad()

	return dbusClient.SetTakeover(enable)
}

// waitHIDHolder polls until some process other than flydigid has one of the controller's
// vendor HID nodes open and keeps it open, returning its name, or "" when the timeout
// expires. A node that is only open briefly doesn't count: that is Steam's driver probing
// the controller and giving up.
func waitHIDHolder(timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for {
		if holder := hidHolder(); holder != "" {
			time.Sleep(1500 * time.Millisecond)
			if again := hidHolder(); again != "" {
				return again
			}
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// recentReconnects returns how many re-enumerations the hotplug handler did during the
// last hotplugGuardInterval.
func recentReconnects() int {
	data, err := os.ReadFile(hotplugGuardFile)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(line)); err == nil && time.Since(t) < hotplugGuardInterval {
			n++
		}
	}
	return n
}

func recordReconnect() {
	os.MkdirAll(filepath.Dir(hotplugGuardFile), 0o755)
	f, err := os.OpenFile(hotplugGuardFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, time.Now().Format(time.RFC3339))
}

// hidHolder returns the name of a process (other than us and flydigid) that has a Flydigi
// v2 hidraw node open, or "".
func hidHolder() string {
	devs, err := hidraw.Find(v2.IsV2Device)
	if err != nil || len(devs) == 0 {
		return ""
	}
	nodes := map[string]bool{}
	for _, d := range devs {
		nodes[d.Path] = true
	}

	self := os.Getpid()
	procs, _ := filepath.Glob("/proc/[0-9]*")
	for _, p := range procs {
		pid, err := strconv.Atoi(filepath.Base(p))
		if err != nil || pid == self {
			continue
		}
		comm, err := os.ReadFile(filepath.Join(p, "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		if name == "flydigid" {
			continue
		}
		fds, err := os.ReadDir(filepath.Join(p, "fd"))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(p, "fd", fd.Name()))
			if err == nil && nodes[target] {
				return name
			}
		}
	}
	return ""
}

// steamRunning reports whether a process named "steam" exists.
func steamRunning() bool {
	procs, _ := filepath.Glob("/proc/[0-9]*/comm")
	for _, p := range procs {
		b, err := os.ReadFile(p)
		if err == nil && strings.TrimSpace(string(b)) == "steam" {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.AddCommand(hotplugCommand)
}
