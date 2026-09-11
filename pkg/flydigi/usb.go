package flydigi

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pipe01/flydigictl/pkg/flydigi/protocol"
	"github.com/rs/zerolog/log"
)

// USBDevicePath returns the sysfs directory of the USB device the gamepad is connected through.
func (g *Gamepad) USBDevicePath() (string, error) {
	loc, ok := g.prot.(protocol.USBLocator)
	if !ok {
		return "", protocol.ErrUnsupported
	}
	p := loc.USBDevicePath()
	if p == "" {
		return "", protocol.ErrUnsupported
	}
	return p, nil
}

// ReenumerateUSB makes the kernel drop and re-enumerate the USB device at the given sysfs
// path (a software "unplug and plug back in"), without power-cycling the controller.
// Applications watching for hotplug events (Steam, SDL) re-detect the controller afterwards.
func ReenumerateUSB(sysPath string) error {
	auth := filepath.Join(sysPath, "authorized")
	if _, err := os.Stat(auth); err != nil {
		return fmt.Errorf("usb device %s: %w", sysPath, err)
	}

	log.Info().Str("device", filepath.Base(sysPath)).Msg("re-enumerating usb device")

	if err := os.WriteFile(auth, []byte("0"), 0); err != nil {
		return fmt.Errorf("deauthorize usb device: %w", err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := os.WriteFile(auth, []byte("1"), 0); err != nil {
		return fmt.Errorf("reauthorize usb device: %w", err)
	}
	return nil
}
