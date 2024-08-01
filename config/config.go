package config

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	_ "github.com/joho/godotenv/autoload"
)

const ConfigFileName = "/data/options.json"

// Config ...
type Config struct {
	Version    string `json:"VERSION"`
	Key        string `json:"KEY"`
	ListenHost string `json:"LISTEN_HOST"`
	ListenPort string `json:"LISTEN_PORT"`
	Debug      bool   `json:"DEBUG"`
}

func InitConfig(args []string, version string) (*Config, error) {
	var config = &Config{
		Version:    version,
		Key:        "",
		ListenHost: "",
		ListenPort: "9056",

		Debug: false,
	}

	var initFromFile = false

	if _, err := os.Stat(ConfigFileName); err == nil {
		jsonFile, err := os.Open(ConfigFileName)
		if err == nil {
			byteValue, _ := io.ReadAll(jsonFile)
			if err = json.Unmarshal(byteValue, &config); err == nil {
				initFromFile = true
			} else {
				fmt.Printf("error on unmarshal config from file %s\n", err.Error())
			}
		}
	}

	if !initFromFile {
		flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
		flags.StringVar(&config.Key, "key", lookupEnvOrString("KEY", config.Key), "KEY")
		flags.StringVar(&config.ListenHost, "listenHost", lookupEnvOrString("LISTEN_HOST", config.ListenHost), "LISTEN_HOST")
		flags.StringVar(&config.ListenPort, "listenPort", lookupEnvOrString("LISTEN_PORT", config.ListenPort), "LISTEN_PORT")
		flags.BoolVar(&config.Debug, "debug", lookupEnvOrBool("DEBUG", config.Debug), "Debug")

		if err := flags.Parse(args[1:]); err != nil {
			return nil, err
		}
	}

	if config.Key == "" {
		key, errGenerateKey := generateKey()
		if errGenerateKey == nil {
			fmt.Printf("you can use this one generated for you: %s\n", key)
		}
		return nil, fmt.Errorf("%s", "key not found in config")
	}

	return config, nil
}

func generateKey() (string, error) {
	_, srvKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(srvKey.Seed()), nil
}
