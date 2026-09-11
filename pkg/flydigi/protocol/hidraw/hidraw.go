// Package hidraw provides pure-Go access to Linux hidraw devices (no cgo, no libusb).
package hidraw

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// DeviceInfo describes a /dev/hidrawN node and its parent USB interface.
type DeviceInfo struct {
	Path      string // e.g. /dev/hidraw6
	Bus       uint16 // HID bus type (3 = USB, 5 = Bluetooth)
	VendorID  uint16
	ProductID uint16
	Name      string
	Interface int // USB interface number, -1 if unknown
	// USBDevice is the sysfs directory of the parent USB device (e.g. /sys/bus/usb/devices/3-4), empty if unknown.
	USBDevice string

	ReportDescriptor []byte
}

// UsagePage returns the first (top-level) usage page declared in the report descriptor.
func (d DeviceInfo) UsagePage() uint16 {
	rd := d.ReportDescriptor
	for i := 0; i < len(rd); {
		prefix := rd[i]
		if prefix == 0xFE { // long item
			if i+1 >= len(rd) {
				break
			}
			i += 3 + int(rd[i+1])
			continue
		}
		size := int(prefix & 0x03)
		if size == 3 {
			size = 4
		}
		tag := prefix & 0xFC
		if tag == 0x04 { // Global: Usage Page
			switch size {
			case 1:
				if i+1 < len(rd) {
					return uint16(rd[i+1])
				}
			case 2:
				if i+2 < len(rd) {
					return uint16(rd[i+1]) | uint16(rd[i+2])<<8
				}
			}
			return 0
		}
		i += 1 + size
	}
	return 0
}

// ReportIDs returns the first and the last report ID declared in the descriptor (0 if none).
// Flydigi Space uses the first as the input report ID and the last as the output report ID.
func (d DeviceInfo) ReportIDs() (first, last byte) {
	rd := d.ReportDescriptor
	for i := 0; i < len(rd); {
		prefix := rd[i]
		if prefix == 0xFE {
			if i+1 >= len(rd) {
				break
			}
			i += 3 + int(rd[i+1])
			continue
		}
		size := int(prefix & 0x03)
		if size == 3 {
			size = 4
		}
		if prefix&0xFC == 0x84 && size >= 1 && i+1 < len(rd) { // Global: Report ID
			if first == 0 {
				first = rd[i+1]
			}
			last = rd[i+1]
		}
		i += 1 + size
	}
	return
}

var ifaceRegexp = regexp.MustCompile(`:\d+\.(\d+)$`)

// Enumerate lists all hidraw devices present in the system.
func Enumerate() ([]DeviceInfo, error) {
	dirs, err := filepath.Glob("/sys/class/hidraw/hidraw*")
	if err != nil {
		return nil, err
	}

	var devs []DeviceInfo

	for _, dir := range dirs {
		info := DeviceInfo{
			Path:      "/dev/" + filepath.Base(dir),
			Interface: -1,
		}

		uevent, err := os.ReadFile(filepath.Join(dir, "device", "uevent"))
		if err != nil {
			continue
		}

		for _, line := range strings.Split(string(uevent), "\n") {
			key, val, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch key {
			case "HID_ID":
				// e.g. 0003:000037D7:00002401
				parts := strings.Split(val, ":")
				if len(parts) == 3 {
					bus, _ := strconv.ParseUint(parts[0], 16, 16)
					vid, _ := strconv.ParseUint(parts[1], 16, 32)
					pid, _ := strconv.ParseUint(parts[2], 16, 32)
					info.Bus = uint16(bus)
					info.VendorID = uint16(vid)
					info.ProductID = uint16(pid)
				}
			case "HID_NAME":
				info.Name = val
			}
		}

		if devPath, err := filepath.EvalSymlinks(filepath.Join(dir, "device")); err == nil {
			// .../usb1/1-1/1-1.1/1-1.1:1.1/0003:37D7:2401.0089
			if m := ifaceRegexp.FindStringSubmatch(filepath.Base(filepath.Dir(devPath))); m != nil {
				info.Interface, _ = strconv.Atoi(m[1])
				info.USBDevice = filepath.Dir(filepath.Dir(devPath))
			}
		}

		info.ReportDescriptor, _ = os.ReadFile(filepath.Join(dir, "device", "report_descriptor"))

		devs = append(devs, info)
	}

	return devs, nil
}

// Find returns the devices matching the given filter.
func Find(match func(DeviceInfo) bool) ([]DeviceInfo, error) {
	devs, err := Enumerate()
	if err != nil {
		return nil, err
	}

	var out []DeviceInfo
	for _, d := range devs {
		if match(d) {
			out = append(out, d)
		}
	}
	return out, nil
}

// Device is an open hidraw node.
type Device struct {
	f *os.File
}

// Open opens a hidraw device node for reading and writing.
func Open(path string) (*Device, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &Device{f: f}, nil
}

// Read reads one input report. For numbered reports the first byte is the report ID.
func (d *Device) Read(p []byte) (int, error) {
	return d.f.Read(p)
}

// Write writes one output report. The first byte must be the report ID (0 for unnumbered reports).
func (d *Device) Write(p []byte) (int, error) {
	return d.f.Write(p)
}

func (d *Device) Close() error {
	return d.f.Close()
}
