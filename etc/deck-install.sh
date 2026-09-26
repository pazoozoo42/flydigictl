#!/bin/bash
# Installs flydigictl/flydigid on a Steam Deck (SteamOS 3.x). Run as root from the
# directory containing the built binaries and the etc/ files.
#
# The payload goes to /opt/flydigictl, which SteamOS keeps across OS updates; everything
# that lives on the root filesystem is (re)created by deck-setup.sh at every boot.
set -euo pipefail

P=/opt/flydigictl
HERE="$(cd "$(dirname "$0")" && pwd)"

install -Dm755 "$HERE/flydigid" "$P/bin/flydigid"
install -Dm755 "$HERE/flydigictl" "$P/bin/flydigictl"
install -Dm755 "$HERE/deck-setup.sh" "$P/deck-setup.sh"
for f in flydigid.service flydigictl-hotplug.service flydigictl-setup.service flydigid.conf 70-flydigi.rules; do
	install -Dm644 "$HERE/$f" "$P/etc/$f"
done

# clean up the old layout (binaries directly in /usr/local/bin, on the read-only rootfs)
reenable_ro=0
for b in flydigid flydigictl; do
	if [ -f "/usr/local/bin/$b" ] && [ ! -L "/usr/local/bin/$b" ]; then
		if [ "$reenable_ro" = 0 ] && steamos-readonly status 2>/dev/null | grep -q enabled; then
			steamos-readonly disable
			reenable_ro=1
		fi
		rm -f "/usr/local/bin/$b" /usr/local/share/dbus-1/system-services/com.pipe01.flydigi.Gamepad.service
	fi
done

"$P/deck-setup.sh" --restart

if [ "$reenable_ro" = 1 ]; then
	steamos-readonly enable
fi

echo "Installed. Try: flydigictl info"
