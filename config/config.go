package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	ApiID   int
	ApiHash string
	Token   string
}

func getEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("environment variable %s is required", name)
	}
	return value, nil
}

func getEnvInt(name string) (int, error) {
	value, err := getEnv(name)
	if err != nil {
		return 0, err
	}

	v, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %s", name, value)
	}

	return v, nil
}

func Load() (*Config, error) {
	apiID, err := getEnvInt("API_ID")
	if err != nil {
		return nil, err
	}

	apiHash, err := getEnv("API_HASH")
	if err != nil {
		return nil, err
	}

	token, err := getEnv("TOKEN")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		ApiID:   apiID,
		ApiHash: apiHash,
		Token:   token,
	}

	return cfg, nil
}
