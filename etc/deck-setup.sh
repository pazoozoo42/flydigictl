#!/bin/bash
# Boot-time / post-update setup for flydigictl on SteamOS. Idempotent.
#
# SteamOS updates replace the whole root filesystem, which wipes /usr/local and parts of
# /etc. The payload therefore lives in /opt/flydigictl (SteamOS offloads /opt to the
# persistent /home partition) and this script, run by flydigictl-setup.service on every
# boot, puts everything else back: systemd units, DBus policy, udev rule, PATH symlinks.
#
# Usage: deck-setup.sh [--restart]   (--restart also restarts flydigid, used after installs)
set -euo pipefail

P=/opt/flydigictl
BIN=$P/bin
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

changed=0
inst() { # src dst
	if ! cmp -s "$1" "$2"; then
		install -Dm644 "$1" "$2"
		changed=1
	fi
}

# systemd units, pointing at the payload
sed "s#^ExecStart=.*flydigid\$#ExecStart=$BIN/flydigid#" "$P/etc/flydigid.service" > "$TMP/flydigid.service"
sed "s#^ExecStart=.*flydigictl hotplug\$#ExecStart=$BIN/flydigictl hotplug#" "$P/etc/flydigictl-hotplug.service" > "$TMP/flydigictl-hotplug.service"
inst "$TMP/flydigid.service" /etc/systemd/system/flydigid.service
inst "$TMP/flydigictl-hotplug.service" /etc/systemd/system/flydigictl-hotplug.service
inst "$P/etc/flydigictl-setup.service" /etc/systemd/system/flydigictl-setup.service
rm -f /etc/systemd/system/flydigictl-takeover.service

# DBus policy (lets root own the bus name and everyone talk to it)
inst "$P/etc/flydigid.conf" /etc/dbus-1/system.d/flydigid.conf

# udev: run `flydigictl hotplug` when a controller is connected
inst "$P/etc/70-flydigi.rules" /etc/udev/rules.d/70-flydigi.rules

# PATH for login shells
printf 'PATH="$PATH:%s"\n' "$BIN" > "$TMP/profile.sh"
inst "$TMP/profile.sh" /etc/profile.d/flydigictl.sh

# Symlinks in /usr/local/bin (on the read-only rootfs; wiped by every SteamOS update)
need_rw=0
for b in flydigid flydigictl; do
	[ "$(readlink /usr/local/bin/$b 2>/dev/null || true)" = "$BIN/$b" ] || need_rw=1
done
if [ "$need_rw" = 1 ]; then
	reenable_ro=0
	if steamos-readonly status 2>/dev/null | grep -q enabled; then
		steamos-readonly disable
		reenable_ro=1
	fi
	mkdir -p /usr/local/bin
	ln -sfn "$BIN/flydigid" /usr/local/bin/flydigid
	ln -sfn "$BIN/flydigictl" /usr/local/bin/flydigictl
	if [ "$reenable_ro" = 1 ]; then
		steamos-readonly enable
	fi
	changed=1
fi

systemctl daemon-reload
systemctl enable flydigictl-setup.service flydigid.service >/dev/null 2>&1 || true
udevadm control --reload

if [ "$changed" = 1 ]; then
	# dbus-broker must reload its policy before flydigid can own its bus name
	systemctl reload dbus.service 2>/dev/null || busctl call org.freedesktop.DBus / org.freedesktop.DBus ReloadConfig || true
fi

if [ "${1:-}" = "--restart" ]; then
	systemctl restart flydigid.service
elif [ "$changed" = 1 ] && systemctl is-active --quiet flydigid.service; then
	systemctl restart flydigid.service
fi

if [ "$changed" = 1 ]; then
	# handle a controller that was already plugged in when the udev rule (re)appeared
	udevadm trigger --action=add --subsystem-match=usb --attr-match=bInterfaceNumber=00 2>/dev/null || true
	echo "flydigictl setup: files (re)installed"
fi
