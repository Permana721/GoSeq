package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	dateRegex     = regexp.MustCompile(`^\d{4}_\d{2}_\d{2}$`)
	filenameRegex = regexp.MustCompile(`^\d{4}_\d{2}_\d{2}\.md$`)
)

type Storage struct {
	dir string
	loc *time.Location
}

func NewStorage(dir string, loc *time.Location) (*Storage, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, err
	}
	return &Storage{
		dir: absDir,
		loc: loc,
	}, nil
}

func (s *Storage) TodayDate() string {
	return time.Now().In(s.loc).Format("2006_01_02")
}

func (s *Storage) FormatDateDisplay(dateStr string) string {
	t, err := time.ParseInLocation("2006_01_02", dateStr, s.loc)
	if err != nil {
		return dateStr
	}
	return t.Format("Monday, 02 January 2006")
}

func (s *Storage) FormatShortDateDisplay(dateStr string) string {
	t, err := time.ParseInLocation("2006_01_02", dateStr, s.loc)
	if err != nil {
		return dateStr
	}
	return t.Format("02 Jan 2006")
}

func (s *Storage) ValidateDate(dateStr string) bool {
	if !dateRegex.MatchString(dateStr) {
		return false
	}
	// Verify it's a valid calendar date
	_, err := time.ParseInLocation("2006_01_02", dateStr, s.loc)
	return err == nil
}

func (s *Storage) getPath(dateStr string) (string, error) {
	if !s.ValidateDate(dateStr) {
		return "", fmt.Errorf("invalid date format: %q", dateStr)
	}

	filename := dateStr + ".md"
	if !filenameRegex.MatchString(filename) {
		return "", fmt.Errorf("invalid filename format: %q", filename)
	}

	fullPath := filepath.Clean(filepath.Join(s.dir, filename))
	rel, err := filepath.Rel(s.dir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel != filename {
		return "", fmt.Errorf("path traversal attempt detected")
	}

	return fullPath, nil
}

func (s *Storage) ReadJournal(dateStr string) (string, error) {
	path, err := s.getPath(dateStr)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	return string(data), nil
}

func (s *Storage) JournalExists(dateStr string) (bool, error) {
	path, err := s.getPath(dateStr)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *Storage) SaveJournal(dateStr, content string) error {
	path, err := s.getPath(dateStr)
	if err != nil {
		return err
	}

	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("failed to generate random suffix for temp file: %w", err)
	}
	tmpPath := filepath.Join(s.dir, fmt.Sprintf(".%s.tmp.%s", dateStr, hex.EncodeToString(randBytes)))

	// Normalize CRLF to LF
	normalized := strings.ReplaceAll(content, "\r\n", "\n")

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	if _, err := f.WriteString(normalized); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to atomic rename file: %w", err)
	}

	return nil
}

func (s *Storage) ListAllDates() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var dates []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filenameRegex.MatchString(name) {
			dateStr := strings.TrimSuffix(name, ".md")
			if s.ValidateDate(dateStr) {
				dates = append(dates, dateStr)
			}
		}
	}

	// Reverse chronological order (newest first)
	sort.Slice(dates, func(i, j int) bool {
		return dates[i] > dates[j]
	})

	return dates, nil
}
