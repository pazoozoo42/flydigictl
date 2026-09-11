DEB_NAME = flydigictl
DEB_VERSION = 0.0.1
DEB_REVISION = 1
DEB_ARCH = amd64

GIT_COMMIT = $(shell git rev-parse --short HEAD)

DEB_FULLNAME = $(DEB_NAME)_$(DEB_VERSION)-$(DEB_REVISION)_$(DEB_ARCH)

SRC := .
GO := go

# Pure Go (hidraw + usbfs), no cgo needed: cross-compiles from anywhere.
export CGO_ENABLED = 0

GO_FLAGS = -ldflags "-X 'github.com/pipe01/flydigictl/pkg/version.Version=$(DEB_VERSION)-$(GIT_COMMIT)'"

bin-daemon:
	$(GO) build $(GO_FLAGS) $(SRC)/cmd/flydigid

bin-ctl:
	$(GO) build $(GO_FLAGS) $(SRC)/cmd/flydigictl

# Cross-compile for a Linux x86_64 target (e.g. Steam Deck) into build/
build-linux:
	mkdir -p build
	GOOS=linux GOARCH=amd64 $(GO) build $(GO_FLAGS) -o build/flydigid $(SRC)/cmd/flydigid
	GOOS=linux GOARCH=amd64 $(GO) build $(GO_FLAGS) -o build/flydigictl $(SRC)/cmd/flydigictl

# Deploy to a Steam Deck over SSH: make deck-deploy DECK=deck@steamdeck (or root@...)
# Non-root users need passwordless sudo or a cached sudo ticket; see README.
DECK ?= deck@steamdeck
DECK_DIR ?= flydigictl-install
deck-deploy: build-linux
	ssh $(DECK) 'mkdir -p ~/$(DECK_DIR)'
	scp build/flydigid build/flydigictl $(SRC)/etc/flydigid.conf $(SRC)/etc/flydigid.service $(SRC)/etc/com.pipe01.flydigi.Gamepad.service \
		$(SRC)/etc/70-flydigi.rules $(SRC)/etc/flydigictl-hotplug.service $(SRC)/etc/deck-install.sh $(DECK):$(DECK_DIR)/
	ssh -t $(DECK) 'if [ "$$(id -u)" = 0 ]; then bash ~/$(DECK_DIR)/deck-install.sh; else sudo bash ~/$(DECK_DIR)/deck-install.sh; fi'

deb-clean:
	rm -rf $(DEB_FULLNAME)

deb: deb-clean bin-daemon bin-ctl
	mkdir -p $(DEB_FULLNAME)/DEBIAN $(DEB_FULLNAME)/usr/bin $(DEB_FULLNAME)/etc/dbus-1/system.d \
			$(DEB_FULLNAME)/usr/lib/systemd/system $(DEB_FULLNAME)/usr/share/dbus-1/system-services \
			$(DEB_FULLNAME)/etc/bash_completion.d $(DEB_FULLNAME)/usr/share/fish/vendor_completions.d

	./flydigictl completion bash > $(DEB_FULLNAME)/etc/bash_completion.d/flydigictl
	./flydigictl completion fish > $(DEB_FULLNAME)/usr/share/fish/vendor_completions.d/flydigictl.fish

	mv flydigid flydigictl $(DEB_FULLNAME)/usr/bin
	cp $(SRC)/etc/flydigid.conf $(DEB_FULLNAME)/etc/dbus-1/system.d
	cp $(SRC)/etc/flydigid.service $(DEB_FULLNAME)/usr/lib/systemd/system
	cp $(SRC)/etc/com.pipe01.flydigi.Gamepad.service $(DEB_FULLNAME)/usr/share/dbus-1/system-services
	mkdir -p $(DEB_FULLNAME)/usr/lib/udev/rules.d
	cp $(SRC)/etc/70-flydigi.rules $(DEB_FULLNAME)/usr/lib/udev/rules.d
	sed 's#/usr/local/bin/flydigictl#/usr/bin/flydigictl#' $(SRC)/etc/flydigictl-hotplug.service > $(DEB_FULLNAME)/usr/lib/systemd/system/flydigictl-hotplug.service

	echo "Package: $(DEB_NAME)" > $(DEB_FULLNAME)/DEBIAN/control
	echo "Version: $(DEB_VERSION)" >> $(DEB_FULLNAME)/DEBIAN/control
	echo "Architecture: $(DEB_ARCH)" >> $(DEB_FULLNAME)/DEBIAN/control
	echo "Maintainer: Felipe Martínez (felipe@pipe01.net)" >> $(DEB_FULLNAME)/DEBIAN/control
	echo "Description: Utility for configuring Flydigi controllers" >> $(DEB_FULLNAME)/DEBIAN/control

	echo "systemctl daemon-reload" >> $(DEB_FULLNAME)/DEBIAN/postinst
	echo "systemctl stop flydigid.service" >> $(DEB_FULLNAME)/DEBIAN/prerm
	echo "systemctl daemon-reload" >> $(DEB_FULLNAME)/DEBIAN/postrm

	chmod 555 $(DEB_FULLNAME)/DEBIAN/postinst $(DEB_FULLNAME)/DEBIAN/prerm $(DEB_FULLNAME)/DEBIAN/postrm

	dpkg-deb --build --root-owner-group $(DEB_FULLNAME)
	rm -rf $(DEB_FULLNAME)

install: bin-daemon bin-ctl
	mv flydigid flydigictl /usr/bin
	cp $(SRC)/etc/flydigid.conf /etc/dbus-1/system.d
	cp $(SRC)/etc/flydigid.service /usr/lib/systemd/system
	cp $(SRC)/etc/com.pipe01.flydigi.Gamepad.service /usr/share/dbus-1/system-services
	cp $(SRC)/etc/70-flydigi.rules /etc/udev/rules.d
	sed 's#/usr/local/bin/flydigictl#/usr/bin/flydigictl#' $(SRC)/etc/flydigictl-hotplug.service > /usr/lib/systemd/system/flydigictl-hotplug.service
	systemctl daemon-reload
	udevadm control --reload-rules
	flydigictl completion bash > /etc/bash_completion.d/flydigictl

uninstall:
	systemctl stop flydigid.service || true
	rm -f /usr/bin/flydigid /usr/bin/flydigictl
	rm -f /etc/dbus-1/system.d/flydigid.conf
	rm -f /usr/lib/systemd/system/flydigid.service
	rm -f /usr/share/dbus-1/system-services/com.pipe01.flydigi.Gamepad.service
	rm -f /etc/udev/rules.d/70-flydigi.rules /usr/lib/systemd/system/flydigictl-hotplug.service
	rm -f /etc/bash_completion.d/flydigictl
	systemctl daemon-reload
