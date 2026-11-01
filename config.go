package main

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"
)

type Config struct {
	Host          string
	Port          string
	JournalsDir   string
	AdminUsername string
	AdminPassword string
	SessionSecret []byte
	SecureCookie  bool
	Location      *time.Location
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}

		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

func LoadConfig() (*Config, error) {
	if err := loadEnvFile(".env"); err != nil {
		log.Printf("Warning: failed to read .env file: %v", err)
	}

	host := getEnv("HOST", "127.0.0.1")
	port := getEnv("PORT", "5000")
	journalsDir := getEnv("JOURNALS_DIR", "./journals")
	adminUsername := getEnv("ADMIN_USERNAME", "admin")
	adminPassword := getEnv("ADMIN_PASSWORD", "admin")
	secureCookie := strings.ToLower(getEnv("SECURE_COOKIE", "false")) == "true"
	tzName := getEnv("TIMEZONE", "Asia/Jakarta")

	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone %s: %w", tzName, err)
	}

	secretStr := os.Getenv("SESSION_SECRET")
	var secretBytes []byte
	if strings.TrimSpace(secretStr) == "" {
		secretBytes = make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, fmt.Errorf("failed to generate random session secret: %w", err)
		}
		log.Printf("Notice: SESSION_SECRET is not set in environment or .env. Using ephemeral in-memory secret (sessions will reset upon server restart).")
	} else {
		secretBytes = []byte(secretStr)
	}

	absJournalsDir, err := filepath.Abs(journalsDir)
	if err != nil {
		return nil, fmt.Errorf("invalid journals directory path: %w", err)
	}
	if err := os.MkdirAll(absJournalsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create journals directory %s: %w", absJournalsDir, err)
	}

	if adminUsername == "admin" && adminPassword == "admin" {
		log.Printf("Notice: Using default admin credentials (admin/admin). Please configure ADMIN_USERNAME and ADMIN_PASSWORD in your .env.")
	}

	return &Config{
		Host:          host,
		Port:          port,
		JournalsDir:   absJournalsDir,
		AdminUsername: adminUsername,
		AdminPassword: adminPassword,
		SessionSecret: secretBytes,
		SecureCookie:  secureCookie,
		Location:      loc,
	}, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}
