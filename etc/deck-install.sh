#!/bin/bash
# Installs flydigictl/flydigid on a Steam Deck (SteamOS 3.x, read-only root).
# Run as root from the directory containing the built binaries and the etc/ files.
set -euo pipefail

BIN=/usr/local/bin
HERE="$(cd "$(dirname "$0")" && pwd)"

reenable_ro=0
if steamos-readonly status 2>/dev/null | grep -q enabled; then
	steamos-readonly disable
	reenable_ro=1
fi

install -Dm755 "$HERE/flydigid" "$BIN/flydigid"
install -Dm755 "$HERE/flydigictl" "$BIN/flydigictl"

# systemd unit (ExecStart points to /usr/local/bin here)
sed "s#/usr/bin/flydigid#$BIN/flydigid#" "$HERE/flydigid.service" > /etc/systemd/system/flydigid.service
install -Dm644 "$HERE/flydigictl-hotplug.service" /etc/systemd/system/flydigictl-hotplug.service
rm -f /etc/systemd/system/flydigictl-takeover.service

# DBus policy + activation
install -Dm644 "$HERE/flydigid.conf" /etc/dbus-1/system.d/flydigid.conf
install -Dm644 "$HERE/com.pipe01.flydigi.Gamepad.service" /usr/local/share/dbus-1/system-services/com.pipe01.flydigi.Gamepad.service

# udev: re-apply takeover setting on hotplug (opt-in via /etc/flydigictl/auto-takeover)
install -Dm644 "$HERE/70-flydigi.rules" /etc/udev/rules.d/70-flydigi.rules

if [ "$reenable_ro" = 1 ]; then
	steamos-readonly enable
fi

systemctl daemon-reload
udevadm control --reload-rules
# dbus-broker must reload its policy to allow flydigid to own its bus name
systemctl reload dbus.service 2>/dev/null || busctl call org.freedesktop.DBus / org.freedesktop.DBus ReloadConfig || true
systemctl restart flydigid.service
systemctl enable flydigid.service >/dev/null 2>&1 || true

echo "Installed. Try: flydigictl info"
