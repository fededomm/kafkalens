// Package config reads the startup settings (config.yaml) with viper.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

type Config struct {
	Server   Server `mapstructure:"server"`
	Clusters string `mapstructure:"clusters"` // "" = default location, resolved by the caller
	Log      Log    `mapstructure:"log"`
}

type Server struct {
	Addr        string `mapstructure:"addr"`
	OpenBrowser bool   `mapstructure:"openBrowser"`
}

type Log struct {
	Level string `mapstructure:"level"`
}

// Load builds the settings, each layer overriding the previous one:
//
//  1. defaults: the embedded config.yaml
//  2. file (skipped when ""), merged key by key; with optional, a missing file is not an error
//  3. environment: KAFKALENS_<SECTION>_<KEY>, e.g. KAFKALENS_LOG_LEVEL, KAFKALENS_SERVER_ADDR
//
// Unknown keys are errors, so typos don't pass silently. source is the file read, or "built-in defaults".
func Load(defaults []byte, file string, optional bool) (c Config, source string, err error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader(defaults)); err != nil {
		return c, "", fmt.Errorf("defaults: %w", err)
	}
	source = "built-in defaults"
	if file != "" {
		fv := viper.New()
		fv.SetConfigFile(file)
		fv.SetConfigType("yaml") // whatever the extension
		switch err := fv.ReadInConfig(); {
		case err == nil:
			// check the file on its own first: merging silently drops a section of the wrong type
			if err := fv.UnmarshalExact(&Config{}); err != nil {
				return c, file, err
			}
			if err := v.MergeConfigMap(fv.AllSettings()); err != nil {
				return c, file, err
			}
			source = file
		case !optional || !errors.Is(err, fs.ErrNotExist):
			return c, file, err
		}
	}
	v.SetEnvPrefix("KAFKALENS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv() // applies to keys known from the defaults

	if err := v.UnmarshalExact(&c); err != nil {
		return c, source, err
	}
	if c.Server.Addr == "" {
		return c, source, errors.New("server.addr is empty")
	}
	if _, err := c.LogLevel(); err != nil {
		return c, source, err
	}
	return c, source, nil
}

// LogLevel is log.level as a zerolog level; only the four documented names are accepted, in any case.
func (c Config) LogLevel() (zerolog.Level, error) {
	switch l := strings.ToLower(c.Log.Level); l {
	case "debug", "info", "warn", "error":
		return zerolog.ParseLevel(l)
	}
	return zerolog.NoLevel, fmt.Errorf("log.level %q: want debug, info, warn or error", c.Log.Level)
}
