package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Article represents a single markdown file in the content directory.
type Article struct {
	Slug        string        // filename without .md
	Section     string        // subfolder name (e.g. "til", "articles")
	Title       string        // from frontmatter or derived from filename
	Date        time.Time     // from frontmatter or file modtime
	DateDisplay string        // formatted: "Oct 27, 2026"
	Lang        string        // "ID" or "EN" (from frontmatter, default "EN")
	Category    string        // from frontmatter (optional)
	RawContent  string        // full file content (after stripping frontmatter)
	HTMLContent template.HTML // rendered from RawContent using outliner parser
	Tags        []string      // extracted from content
	PageLinks   []string      // extracted from content
}

// Section represents a content subdirectory with its articles.
type Section struct {
	Name        string     // e.g. "til", "articles"
	DisplayName string     // e.g. "Today I Learned", "Articles"
	Description string     // e.g. "Bite-sized notes, daily discoveries, and quick learnings."
	Articles    []*Article // sorted newest first
}

// sectionDisplayNames maps subfolder names to human-readable display names.
var sectionDisplayNames = map[string]string{
	"til":      "Today I Learned",
	"articles": "Articles",
}

// sectionDescriptions provides brief summaries for the category cards.
var sectionDescriptions = map[string]string{
	"articles": "Long-form writing, technical deep-dives, and essays.",
	"til":      "Bite-sized notes, daily discoveries, and quick technical learnings.",
}

func sectionDisplayName(name string) string {
	if dn, ok := sectionDisplayNames[name]; ok {
		return dn
	}
	// Default: title-case the folder name
	return toTitleCase(strings.ReplaceAll(name, "_", " "))
}

func sectionDescription(name string) string {
	if desc, ok := sectionDescriptions[name]; ok {
		return desc
	}
	return "Explore notes and writings in this section."
}

// ContentStore manages the content directory.
type ContentStore struct {
	dir string
	loc *time.Location
}

// NewContentStore creates a ContentStore and ensures required subdirectories exist.
func NewContentStore(contentDir string, loc *time.Location) (*ContentStore, error) {
	absDir, err := filepath.Abs(contentDir)
	if err != nil {
		return nil, err
	}
	// Create base directory and default subdirs
	for _, sub := range []string{"", "til", "articles"} {
		p := filepath.Join(absDir, sub)
		if err := os.MkdirAll(p, 0755); err != nil {
			return nil, fmt.Errorf("failed to create content directory %s: %w", p, err)
		}
	}
	return &ContentStore{dir: absDir, loc: loc}, nil
}

// TodayDate returns today's date in "YYYY_MM_DD" format.
func (cs *ContentStore) TodayDate() string {
	return time.Now().In(cs.loc).Format("2006_01_02")
}

// ListAllSections returns all subfolder names under the content directory.
func (cs *ContentStore) ListAllSections() ([]string, error) {
	entries, err := os.ReadDir(cs.dir)
	if err != nil {
		return nil, err
	}
	var sections []string
	for _, e := range entries {
		if e.IsDir() {
			sections = append(sections, e.Name())
		}
	}
	sort.Strings(sections)
	return sections, nil
}

// SectionExists reports whether a subdirectory exists under the content directory.
func (cs *ContentStore) SectionExists(section string) bool {
	sectionPath, err := cs.safeSectionPath(section)
	if err != nil {
		return false
	}
	info, err := os.Stat(sectionPath)
	return err == nil && info.IsDir()
}

// ListSections returns all sections with their articles loaded (newest first).
func (cs *ContentStore) ListSections() ([]Section, error) {
	sectionNames, err := cs.ListAllSections()
	if err != nil {
		return nil, err
	}
	var sections []Section
	for _, name := range sectionNames {
		articles, err := cs.ListArticlesInSection(name)
		if err != nil {
			return nil, err
		}
		sections = append(sections, Section{
			Name:        name,
			DisplayName: sectionDisplayName(name),
			Description: sectionDescription(name),
			Articles:    articles,
		})
	}
	return sections, nil
}

// ListArticlesInSection returns all articles in a section sorted newest first.
func (cs *ContentStore) ListArticlesInSection(section string) ([]*Article, error) {
	sectionPath, err := cs.safeSectionPath(section)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(sectionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var articles []*Article
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		article, err := cs.GetArticle(section, slug)
		if err != nil {
			continue
		}
		articles = append(articles, article)
	}

	// Sort newest first
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].Date.After(articles[j].Date)
	})

	return articles, nil
}

// GetArticle reads and parses a single article file.
func (cs *ContentStore) GetArticle(section, slug string) (*Article, error) {
	filePath, err := cs.safeArticlePath(section, slug)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("article not found: %s/%s", section, slug)
		}
		return nil, err
	}

	// Get file modtime for fallback date
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	modTime := info.ModTime().In(cs.loc)

	raw := string(data)
	fm, body := parseFrontmatter(raw)

	// Resolve title
	title := fm["Title"]
	if title == "" {
		title = slugToTitle(slug)
	}

	// Resolve date
	var date time.Time
	if dateStr, ok := fm["Date"]; ok && dateStr != "" {
		t, err := time.ParseInLocation("2006-01-02", dateStr, cs.loc)
		if err == nil {
			date = t
		} else {
			date = modTime
		}
	} else {
		date = modTime
	}

	// Resolve lang
	lang := fm["Lang"]
	if lang == "" {
		lang = "EN"
	}
	lang = strings.ToUpper(lang)

	category := fm["Category"]

	// Parse content
	htmlContent, tags, pageLinks := ParseContent(body)

	return &Article{
		Slug:        slug,
		Section:     section,
		Title:       title,
		Date:        date,
		DateDisplay: date.Format("Jan 02, 2006"),
		Lang:        lang,
		Category:    category,
		RawContent:  body,
		HTMLContent: htmlContent,
		Tags:        tags,
		PageLinks:   pageLinks,
	}, nil
}

// SaveArticle atomically saves an article's raw content to disk.
func (cs *ContentStore) SaveArticle(section, slug, rawContent string) error {
	filePath, err := cs.safeArticlePath(section, slug)
	if err != nil {
		return err
	}

	// Normalize CRLF to LF
	normalized := strings.ReplaceAll(rawContent, "\r\n", "\n")

	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("failed to generate random suffix: %w", err)
	}
	tmpPath := filepath.Join(filepath.Dir(filePath), fmt.Sprintf(".%s.tmp.%s", slug, hex.EncodeToString(randBytes)))

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

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// --- Path helpers ---

func (cs *ContentStore) safeSectionPath(section string) (string, error) {
	if strings.ContainsAny(section, "/\\") || strings.Contains(section, "..") || section == "" {
		return "", fmt.Errorf("invalid section name: %q", section)
	}
	p := filepath.Clean(filepath.Join(cs.dir, section))
	rel, err := filepath.Rel(cs.dir, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path traversal attempt in section: %q", section)
	}
	return p, nil
}

func (cs *ContentStore) safeArticlePath(section, slug string) (string, error) {
	sectionPath, err := cs.safeSectionPath(section)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(slug, "/\\") || strings.Contains(slug, "..") || slug == "" {
		return "", fmt.Errorf("invalid slug: %q", slug)
	}
	filename := slug + ".md"
	fullPath := filepath.Clean(filepath.Join(sectionPath, filename))
	rel, err := filepath.Rel(sectionPath, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel != filename {
		return "", fmt.Errorf("path traversal attempt in slug: %q", slug)
	}
	return fullPath, nil
}

// --- Frontmatter parsing ---

// parseFrontmatter splits YAML-like frontmatter from body content.
// Frontmatter is between the first two "---" delimiters at the top of the file.
// Returns (fields map, body string).
func parseFrontmatter(raw string) (map[string]string, string) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	fields := make(map[string]string)

	// Must start with "---"
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fields, raw
	}

	// Find the closing "---"
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		// No closing delimiter; treat as no frontmatter
		return fields, raw
	}

	// Parse fields
	for i := 1; i < closeIdx; i++ {
		line := lines[i]
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key != "" {
			fields[key] = val
		}
	}

	// Body is everything after the closing "---"
	body := strings.Join(lines[closeIdx+1:], "\n")
	body = strings.TrimLeft(body, "\n")
	return fields, body
}

// --- Title helpers ---

// slugToTitle converts a slug like "golang_tips" or "belajar-go" to "Golang Tips" / "Belajar Go".
func slugToTitle(slug string) string {
	s := strings.ReplaceAll(slug, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	return toTitleCase(s)
}

func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		runes := []rune(w)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}
