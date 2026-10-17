// SPDX-License-Identifier: GPL-3.0-or-later

// Command gamepad-desktop turns a gamepad into a keyboard and mouse.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/LeVraiBaptiste/gamepad-desktop/internal/config"
	"github.com/LeVraiBaptiste/gamepad-desktop/internal/gnomeext"
)

// version is set at build time with -ldflags "-X main.version=...".
var version string

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version // set by go install
	}
	return "unknown"
}

func main() {
	log.SetFlags(0)
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install-extension":
			dir, err := gnomeext.Install()
			if err != nil {
				log.Fatalf("installing the GNOME extension: %v", err)
			}
			log.Printf("GNOME extension installed in %s and enabled.", dir)
			log.Printf("Log out and back in so GNOME Shell loads it (Wayland cannot load it in a running session).")
			return
		case "uninstall-extension":
			dir, err := gnomeext.Uninstall()
			if err != nil {
				log.Fatalf("uninstalling the GNOME extension: %v", err)
			}
			log.Printf("GNOME extension disabled and removed from %s.", dir)
			return
		}
	}

	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), `Usage:
  gamepad-desktop [flags]              run the daemon
  gamepad-desktop install-extension    install the GNOME extension that reports the focused app
  gamepad-desktop uninstall-extension  remove it

Flags:
`)
		flag.PrintDefaults()
	}
	cfgPath := flag.String("config", "", "config file (default: search $XDG_CONFIG_HOME, then /etc)")
	check := flag.Bool("check", false, "validate the config and exit")
	devPath := flag.String("device", "", "gamepad event device (default: auto-detect)")
	verbose := flag.Bool("verbose", false, "log every gamepad event and the action it triggers")
	enableExt := flag.Bool("enable-extension", false, "make sure the GNOME extension is enabled, keeping the other extensions (for packaged installs)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", flag.Arg(0))
		flag.Usage()
		os.Exit(2)
	}
	if *showVersion {
		fmt.Println("gamepad-desktop", buildVersion())
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Path == "" {
		log.Printf("config: none found in %v, using the built-in one", config.Paths())
	} else {
		log.Printf("config: %s", cfg.Path)
	}
	if *check {
		log.Printf("config OK: default profile + %d app profile(s)", len(cfg.Apps))
		return
	}

	if *enableExt {
		if changed, err := gnomeext.Enable(); err != nil {
			log.Printf("GNOME extension: %v", err)
		} else if changed {
			log.Printf("GNOME extension enabled; GNOME Shell loads it at the next login")
		}
	}

	if err := run(cfg, *devPath, *verbose); err != nil {
		log.Fatal(err)
	}
}
