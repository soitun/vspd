// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/hdkeychain/v3"
	"github.com/decred/slog"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/internal/config"
	"github.com/decred/vspd/internal/vspd"
	"github.com/jessevdk/go-flags"
)

const (
	configFilename = "vspd.conf"
	dbFilename     = "vspd.db"
)

type conf struct {
	HomeDir string `long:"homedir" description:"Path to application home directory."`
	Network string `long:"network" description:"Decred network to use." choice:"mainnet" choice:"testnet" choice:"simnet"`
}

var defaultConf = conf{
	HomeDir: dcrutil.AppDataDir("vspd", false),
	Network: "mainnet",
}

func log(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

// fileExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); os.IsNotExist(err) {
		return false
	}
	return true
}

// validatePubkey returns an error if the provided key is invalid, not for the
// expected network, or it is public instead of private.
func validatePubkey(key string, network *config.Network) error {
	parsedKey, err := hdkeychain.NewKeyFromString(key, network.Params)
	if err != nil {
		return fmt.Errorf("failed to parse feexpub: %w", err)
	}

	if parsedKey.IsPrivate() {
		return errors.New("feexpub is a private key, should be public")
	}

	return nil
}

func createDatabase(homeDir string, feeXPub string, network *config.Network) error {
	dataDir := filepath.Join(homeDir, "data", network.Name)
	dbFile := filepath.Join(dataDir, dbFilename)

	// Return error if database already exists.
	if fileExists(dbFile) {
		return fmt.Errorf("%s database already exists in %s", network.Name, dataDir)
	}

	// Ensure provided xpub is a valid key for the selected network.
	err := validatePubkey(feeXPub, network)
	if err != nil {
		return err
	}

	// Ensure the data directory exists.
	err = os.MkdirAll(dataDir, 0700)
	if err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create new database.
	err = database.CreateNew(dbFile, feeXPub)
	if err != nil {
		return fmt.Errorf("error creating db file %s: %w", dbFile, err)
	}

	return nil
}

func writeConfig(homeDir string) error {
	configFile := filepath.Join(homeDir, configFilename)

	// Return an error if the config file already exists.
	if fileExists(configFile) {
		return fmt.Errorf("config file already exists in %s", homeDir)
	}

	// Ensure the directory exists.
	err := os.MkdirAll(homeDir, 0700)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write a config file with default values to the provided home directory.
	preParser := flags.NewParser(&vspd.DefaultConfig, flags.None)
	preIni := flags.NewIniParser(preParser)
	err = preIni.WriteFile(configFile,
		flags.IniIncludeComments|flags.IniIncludeDefaults)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	return nil
}

func retireXPub(homeDir string, feeXPub string, network *config.Network) error {
	dataDir := filepath.Join(homeDir, "data", network.Name)
	dbFile := filepath.Join(dataDir, dbFilename)

	// Ensure provided xpub is a valid key for the selected network.
	err := validatePubkey(feeXPub, network)
	if err != nil {
		return err
	}

	db, err := database.Open(dbFile, slog.Disabled, 999)
	if err != nil {
		return fmt.Errorf("error opening db file %s: %w", dbFile, err)
	}

	err = db.RetireXPub(feeXPub)
	if err != nil {
		return fmt.Errorf("db.RetireXPub failed: %w", err)
	}

	return nil
}

// run is the real main function for vspadmin. It is necessary to work around
// the fact that deferred functions do not run when os.Exit() is called.
func run() int {
	cfg := defaultConf

	// If command line options are requesting help, write it to stdout and exit.
	if config.WriteHelp(&cfg) {
		return 0
	}

	// Parse command line options.
	remainingArgs, err := flags.Parse(&cfg)
	if err != nil {
		// Don't need to log the error, flags lib has already done it.
		return 1
	}

	network, err := config.NetworkFromName(cfg.Network)
	if err != nil {
		log("%v", err)
		return 1
	}

	if len(remainingArgs) < 1 {
		log("No command specified")
		return 1
	}

	switch remainingArgs[0] {
	case "createdatabase":
		if len(remainingArgs) != 2 {
			log("createdatabase has one required argument, fee xpub")
			return 1
		}

		feeXPub := remainingArgs[1]

		err = createDatabase(cfg.HomeDir, feeXPub, network)
		if err != nil {
			log("createdatabase failed: %v", err)
			return 1
		}

		log("New %s vspd database created in %s", network.Name, cfg.HomeDir)

	case "writeconfig":
		err = writeConfig(cfg.HomeDir)
		if err != nil {
			log("writeconfig failed: %v", err)
			return 1
		}

		log("Config file with default values written to %s", cfg.HomeDir)
		log("Edit the file and fill in values specific to your vspd deployment")

	case "retirexpub":
		if len(remainingArgs) != 2 {
			log("retirexpub has one required argument, fee xpub")
			return 1
		}

		feeXPub := remainingArgs[1]

		err = retireXPub(cfg.HomeDir, feeXPub, network)
		if err != nil {
			log("retirexpub failed: %v", err)
			return 1
		}

		log("Xpub successfully retired, all future tickets will use the new xpub")

	default:
		log("%q is not a valid command", remainingArgs[0])
		return 1
	}

	return 0
}

func main() {
	os.Exit(run())
}
