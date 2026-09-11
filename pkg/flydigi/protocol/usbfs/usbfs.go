// Package usbfs provides minimal pure-Go access to USB devices through Linux usbfs
// (/dev/bus/usb), enough to talk to vendor-specific interrupt endpoints without libusb.
package usbfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ioctl request numbers (asm-generic ioctl encoding, identical on amd64 and arm64).
const (
	usbdevfsBulk             = 0xc0185502 // _IOWR('U', 2, struct usbdevfs_bulktransfer)
	usbdevfsClaimInterface   = 0x8004550f // _IOR('U', 15, unsigned int)
	usbdevfsReleaseInterface = 0x80045510 // _IOR('U', 16, unsigned int)
	usbdevfsIoctl            = 0xc0105512 // _IOWR('U', 18, struct usbdevfs_ioctl)
	usbdevfsDisconnect       = 0x00005516 // _IO('U', 22)
	usbdevfsConnect          = 0x00005517 // _IO('U', 23)
)

type bulkTransfer struct {
	Ep      uint32
	Len     uint32
	Timeout uint32
	_       uint32
	Data    unsafe.Pointer
}

type ioctlArg struct {
	Ifno      int32
	IoctlCode int32
	Data      unsafe.Pointer
}

// DeviceInfo describes a USB device found in sysfs.
type DeviceInfo struct {
	Path      string // /dev/bus/usb/BBB/DDD
	SysPath   string
	VendorID  uint16
	ProductID uint16
}

func readHex16(path string) (uint16, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 16, 16)
	return uint16(v), err
}

func readInt(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

// Enumerate lists USB devices (not interfaces) present in the system.
func Enumerate() ([]DeviceInfo, error) {
	entries, err := filepath.Glob("/sys/bus/usb/devices/*")
	if err != nil {
		return nil, err
	}

	var devs []DeviceInfo

	for _, sysPath := range entries {
		if strings.Contains(filepath.Base(sysPath), ":") { // interface, not device
			continue
		}

		vid, err := readHex16(filepath.Join(sysPath, "idVendor"))
		if err != nil {
			continue
		}
		pid, err := readHex16(filepath.Join(sysPath, "idProduct"))
		if err != nil {
			continue
		}
		bus, err := readInt(filepath.Join(sysPath, "busnum"))
		if err != nil {
			continue
		}
		dev, err := readInt(filepath.Join(sysPath, "devnum"))
		if err != nil {
			continue
		}

		devs = append(devs, DeviceInfo{
			Path:      fmt.Sprintf("/dev/bus/usb/%03d/%03d", bus, dev),
			SysPath:   sysPath,
			VendorID:  vid,
			ProductID: pid,
		})
	}

	return devs, nil
}

// Find returns the devices matching the given vendor and product ID.
func Find(vid, pid uint16) ([]DeviceInfo, error) {
	devs, err := Enumerate()
	if err != nil {
		return nil, err
	}

	var out []DeviceInfo
	for _, d := range devs {
		if d.VendorID == vid && d.ProductID == pid {
			out = append(out, d)
		}
	}
	return out, nil
}

// Device is an open usbfs device with one claimed interface.
type Device struct {
	fd       int
	iface    int
	detached bool
}

func ioctl(fd int, req uint, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

// Open opens the device node, detaches any kernel driver bound to the interface
// (e.g. xpad) and claims the interface.
func Open(path string, iface int) (*Device, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	d := &Device{fd: fd, iface: iface}

	// Detach the kernel driver (ENODATA means no driver was bound).
	arg := ioctlArg{Ifno: int32(iface), IoctlCode: usbdevfsDisconnect}
	if err := ioctl(fd, usbdevfsIoctl, unsafe.Pointer(&arg)); err == nil {
		d.detached = true
	} else if !errors.Is(err, unix.ENODATA) {
		unix.Close(fd)
		return nil, fmt.Errorf("detach kernel driver: %w", err)
	}

	ifno := uint32(iface)
	if err := ioctl(fd, usbdevfsClaimInterface, unsafe.Pointer(&ifno)); err != nil {
		if d.detached {
			ioctl(fd, usbdevfsIoctl, unsafe.Pointer(&ioctlArg{Ifno: int32(iface), IoctlCode: usbdevfsConnect}))
		}
		unix.Close(fd)
		return nil, fmt.Errorf("claim interface %d: %w", iface, err)
	}

	return d, nil
}

// Close releases the interface, re-attaches the kernel driver if one was detached, and closes the device.
func (d *Device) Close() error {
	ifno := uint32(d.iface)
	ioctl(d.fd, usbdevfsReleaseInterface, unsafe.Pointer(&ifno))

	if d.detached {
		ioctl(d.fd, usbdevfsIoctl, unsafe.Pointer(&ioctlArg{Ifno: int32(d.iface), IoctlCode: usbdevfsConnect}))
	}

	return unix.Close(d.fd)
}

// Transfer performs a bulk/interrupt transfer on the given endpoint address
// (bit 7 set = IN). It returns the number of bytes transferred. A timeout is
// reported as unix.ETIMEDOUT.
func (d *Device) Transfer(ep byte, data []byte, timeoutMs uint32) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	t := bulkTransfer{
		Ep:      uint32(ep),
		Len:     uint32(len(data)),
		Timeout: timeoutMs,
		Data:    unsafe.Pointer(&data[0]),
	}

	n, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(d.fd), uintptr(usbdevfsBulk), uintptr(unsafe.Pointer(&t)))
	if errno != 0 {
		return 0, errno
	}

	return int(n), nil
}

// IsTimeout reports whether err is a transfer timeout.
func IsTimeout(err error) bool {
	return errors.Is(err, unix.ETIMEDOUT)
}

// IsNoDevice reports whether err indicates the device was disconnected.
func IsNoDevice(err error) bool {
	return errors.Is(err, unix.ENODEV) || errors.Is(err, unix.ENOENT)
}
