package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fran/piensa/pkg/models"
)

const (
	configDir    = ".config/piensa"
	configFile   = "config.json"
	envConfigPath = "PIENSA_CONFIG_PATH"
)

func configPath() (string, error) {
	if p := os.Getenv(envConfigPath); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, configDir, configFile), nil
}

func Load() (*models.Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg models.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func Save(cfg *models.Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func FindAccountByServerID(cfg *models.Config, serverID string) (*models.Account, *models.ServerToken) {
	for i := range cfg.Accounts {
		for j := range cfg.Accounts[i].Servers {
			st := &cfg.Accounts[i].Servers[j]
			if st.ServerID == serverID || strings.HasPrefix(st.ServerID, serverID) {
				return &cfg.Accounts[i], st
			}
		}
	}
	return nil, nil
}


