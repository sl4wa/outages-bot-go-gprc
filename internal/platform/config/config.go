package config

import (
	"fmt"
	"os"
	"time"
)

// String returns a required environment variable.
func String(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

// Duration returns a required nonnegative duration; 0 is allowed.
func Duration(key string) (time.Duration, error) {
	value, err := String(key)
	if err != nil {
		return 0, err
	}
	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s must be a nonnegative duration", key)
	}
	return d, nil
}
