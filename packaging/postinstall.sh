#!/bin/sh
# Load uinput now (modules-load.d covers the next boots) and apply the udev
# rule to the devices already present, so no reboot is needed.
modprobe uinput 2>/dev/null || true
if command -v udevadm >/dev/null 2>&1; then
	udevadm control --reload || true
	udevadm trigger --subsystem-match=misc --subsystem-match=input || true
fi
