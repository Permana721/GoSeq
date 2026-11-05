package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

type Article struct {
	Slug        string
	Section     string
	Title       string
	Date        time.Time
	DateDisplay string
	Lang        string
	Category    string
	RawContent  string
	HTMLContent template.HTML
	Tags        []string
	PageLinks   []string
	ReadingTime string
}

type Section struct {
	Name        string
	DisplayName string
	Description string
	Articles    []*Article
}

var sectionDisplayNames = map[string]string{
	"til":      "Today I Learned",
	"articles": "Articles",
}

var sectionDescriptions = map[string]string{
	"articles": "Thoughts, stories, and knowledge I want to share from my experiences.",
	"til":      "Short notes on things I learned today and small daily discoveries.",
}

func sectionDisplayName(name string) string {
	if dn, ok := sectionDisplayNames[name]; ok {
		return dn
	}
	return toTitleCase(strings.ReplaceAll(name, "_", " "))
}

func sectionDescription(name string) string {
	if desc, ok := sectionDescriptions[name]; ok {
		return desc
	}
	return "Explore notes and writings in this section."
}

type ContentStore struct {
	dir string
	loc *time.Location
}

func NewContentStore(contentDir string, loc *time.Location) (*ContentStore, error) {
	absDir, err := filepath.Abs(contentDir)
	if err != nil {
		return nil, err
	}
	for _, sub := range []string{"", "til", "articles", "uploads"} {
		p := filepath.Join(absDir, sub)
		if err := os.MkdirAll(p, 0755); err != nil {
			return nil, fmt.Errorf("failed to create content directory %s: %w", p, err)
		}
	}
	return &ContentStore{dir: absDir, loc: loc}, nil
}

func (cs *ContentStore) TodayDate() string {
	return time.Now().In(cs.loc).Format("2006_01_02")
}

func (cs *ContentStore) ListAllSections() ([]string, error) {
	entries, err := os.ReadDir(cs.dir)
	if err != nil {
		return nil, err
	}
	var sections []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "uploads" && !strings.HasPrefix(e.Name(), ".") {
			sections = append(sections, e.Name())
		}
	}
	sort.Strings(sections)
	return sections, nil
}

func (cs *ContentStore) SectionExists(section string) bool {
	sectionPath, err := cs.safeSectionPath(section)
	if err != nil {
		return false
	}
	info, err := os.Stat(sectionPath)
	return err == nil && info.IsDir()
}

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

	sort.Slice(articles, func(i, j int) bool {
		return articles[i].Date.After(articles[j].Date)
	})

	return articles, nil
}

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

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	modTime := info.ModTime().In(cs.loc)

	raw := string(data)
	fm, body := parseFrontmatter(raw)

	title := fm["Title"]
	if title == "" {
		if outlinerTitle := extractOutlinerTitle(body); outlinerTitle != "" {
			title = outlinerTitle
		} else {
			title = slugToTitle(slug)
		}
	}

	var date time.Time
	if dateStr, ok := fm["Date"]; ok && dateStr != "" {
		if t, ok := parseDateString(dateStr, cs.loc); ok {
			date = t
		} else {
			date = modTime
		}
	} else if dateStr := extractOutlinerDate(body); dateStr != "" {
		if t, ok := parseDateString(dateStr, cs.loc); ok {
			date = t
		} else {
			date = modTime
		}
	} else if t, ok := parseSlugDate(slug, cs.loc); ok {
		date = t
	} else {
		date = modTime
	}

	lang := fm["Lang"]
	if lang == "" {
		lang = extractOutlinerLang(body)
	}
	if lang == "" {
		lang = "EN"
	}
	lang = strings.ToUpper(lang)

	category := fm["Category"]
	if category == "" {
		category = extractOutlinerCategory(body)
	}

	var htmlContent template.HTML
	var tags, pageLinks []string
	var readingTime string

	if section == "articles" || section == "til" {
		htmlContent, tags, pageLinks = ParseArticle(body)
		readingTime = calculateReadingTime(body)
	} else {
		htmlContent, tags, pageLinks = ParseContent(body)
	}

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
		ReadingTime: readingTime,
	}, nil
}

type ActivityHeatmap struct {
	TotalContributions int
	StartDate          string
	EndDate            string
	Weeks              []HeatmapWeek
	Months             []HeatmapMonth
	ActivityCounts     map[string]int
}

type HeatmapWeek struct {
	Days []HeatmapDay
}

type HeatmapDay struct {
	DateStr      string              `json:"date"`
	Display      string              `json:"display"`
	Count        int                 `json:"count"`
	Level        int                 `json:"level"`
	IsFuture     bool                `json:"is_future"`
	Link         string              `json:"link,omitempty"`
	Articles     []HeatmapArticleRef `json:"articles,omitempty"`
	ArticlesJSON string              `json:"articles_json,omitempty"`
}

type HeatmapArticleRef struct {
	Section string `json:"section"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Link    string `json:"link"`
}

type HeatmapMonth struct {
	Name     string
	ColStart int
}

func (cs *ContentStore) GetActivityData() (map[string]int, map[string][]HeatmapArticleRef) {
	counts := make(map[string]int)
	articlesByDate := make(map[string][]HeatmapArticleRef)

	sections, err := cs.ListAllSections()
	if err != nil || len(sections) == 0 {
		sections = []string{"articles", "til"}
	}

	for _, section := range sections {
		articles, err := cs.ListArticlesInSection(section)
		if err != nil {
			continue
		}
		for _, art := range articles {
			dateStr := art.Date.In(cs.loc).Format("2006-01-02")
			counts[dateStr]++
			ref := HeatmapArticleRef{
				Section: art.Section,
				Slug:    art.Slug,
				Title:   art.Title,
				Link:    fmt.Sprintf("/read/%s/%s", art.Section, art.Slug),
			}
			articlesByDate[dateStr] = append(articlesByDate[dateStr], ref)
		}
	}

	return counts, articlesByDate
}

func (cs *ContentStore) BuildHeatmap() *ActivityHeatmap {
	counts, articlesByDate := cs.GetActivityData()

	now := time.Now().In(cs.loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cs.loc)

	weekday := int(today.Weekday())
	endSaturday := today.AddDate(0, 0, 6-weekday)

	startDate := endSaturday.AddDate(0, 0, -(53*7 - 1))

	var weeks []HeatmapWeek
	totalContribs := 0
	currentDate := startDate

	for w := 0; w < 53; w++ {
		var week HeatmapWeek
		for d := 0; d < 7; d++ {
			dateStr := currentDate.Format("2006-01-02")
			display := currentDate.Format("Jan 02, 2006")
			isFuture := currentDate.After(today)

			count := 0
			level := 0
			var articles []HeatmapArticleRef
			articlesJSON := "[]"
			link := ""

			if !isFuture {
				count = counts[dateStr]
				articles = articlesByDate[dateStr]
				totalContribs += count

				if count >= 4 {
					level = 4
				} else if count == 3 {
					level = 3
				} else if count == 2 {
					level = 2
				} else if count == 1 {
					level = 1
				}

				if len(articles) > 0 {
					link = articles[0].Link
					if b, err := json.Marshal(articles); err == nil {
						articlesJSON = string(b)
					}
				}
			}

			week.Days = append(week.Days, HeatmapDay{
				DateStr:      dateStr,
				Display:      display,
				Count:        count,
				Level:        level,
				IsFuture:     isFuture,
				Link:         link,
				Articles:     articles,
				ArticlesJSON: articlesJSON,
			})

			currentDate = currentDate.AddDate(0, 0, 1)
		}
		weeks = append(weeks, week)
	}

	var months []HeatmapMonth
	lastMonthLabelCol := -10
	for w := 0; w < len(weeks); w++ {
		firstOfMonthFound := false
		var mName string
		for d := 0; d < 7; d++ {
			t, err := time.ParseInLocation("2006-01-02", weeks[w].Days[d].DateStr, cs.loc)
			if err != nil {
				continue
			}
			if t.Day() == 1 || (w == 0 && d == 0) {
				firstOfMonthFound = true
				mName = t.Format("Jan")
				break
			}
		}
		if firstOfMonthFound && (w-lastMonthLabelCol >= 3) {
			months = append(months, HeatmapMonth{
				Name:     mName,
				ColStart: w + 1,
			})
			lastMonthLabelCol = w
		}
	}

	return &ActivityHeatmap{
		TotalContributions: totalContribs,
		StartDate:          startDate.Format("Jan 02, 2006"),
		EndDate:            today.Format("Jan 02, 2006"),
		Weeks:              weeks,
		Months:             months,
		ActivityCounts:     counts,
	}
}

func (cs *ContentStore) ComputeVerifiedDate(section, slug string) time.Time {
	if article, err := cs.GetArticle(section, slug); err == nil {
		if !article.Date.IsZero() {
			return article.Date.In(cs.loc)
		}
	}

	if t, ok := parseSlugDate(slug, cs.loc); ok {
		return t.In(cs.loc)
	}

	return time.Now().In(cs.loc)
}

func extractOutlinerTitle(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if outlinerDateRegex.MatchString(trimmed) || outlinerLangRegex.MatchString(trimmed) {
			continue
		}
		if strings.HasPrefix(trimmed, "- # ") || strings.HasPrefix(trimmed, "* # ") {
			title := strings.TrimSpace(trimmed[4:])
			if strings.EqualFold(title, "HEADER / TITLE") || title == "" {
				return ""
			}
			return title
		}
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimSpace(trimmed[2:])
			if strings.EqualFold(title, "HEADER / TITLE") || title == "" {
				return ""
			}
			return title
		}
		break
	}
	return ""
}

var outlinerDateRegex = regexp.MustCompile(`(?i)^\s*(?:[-*]\s*)?Date::\s*(.+)$`)
var outlinerLangRegex = regexp.MustCompile(`(?i)^\s*(?:[-*]\s*)?lang::\s*(.+)$`)
var outlinerCategoryRegex = regexp.MustCompile(`(?i)^\s*(?:[-*]\s*)?(?:category|topic)::\s*(.+)$`)

func extractOutlinerDate(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for _, line := range lines {
		m := outlinerDateRegex.FindStringSubmatch(line)
		if len(m) == 2 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func extractOutlinerLang(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	limit := len(lines)
	if limit > 20 {
		limit = 20
	}
	for _, line := range lines[:limit] {
		m := outlinerLangRegex.FindStringSubmatch(line)
		if len(m) == 2 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func extractOutlinerCategory(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	limit := len(lines)
	if limit > 20 {
		limit = 20
	}
	for _, line := range lines[:limit] {
		m := outlinerCategoryRegex.FindStringSubmatch(line)
		if len(m) == 2 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func parseDateString(dateStr string, loc *time.Location) (time.Time, bool) {
	dateStr = strings.TrimSpace(dateStr)
	dateStr = strings.TrimPrefix(dateStr, "[[")
	dateStr = strings.TrimSuffix(dateStr, "]]")
	dateStr = strings.TrimSpace(dateStr)

	formats := []string{
		"Monday, 02 January 2006",
		"Monday, 2 January 2006",
		"2006-01-02",
		"2006_01_02",
		"02 January 2006",
		"2 January 2006",
		"Jan 02, 2006",
		"January 02, 2006",
		"January 2, 2006",
		"2006-01-02 15:04:05",
	}

	for _, f := range formats {
		if t, err := time.ParseInLocation(f, dateStr, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseSlugDate(slug string, loc *time.Location) (time.Time, bool) {
	slug = strings.TrimSpace(slug)
	if len(slug) >= 10 {
		prefix := slug[:10]
		if t, err := time.ParseInLocation("2006_01_02", prefix, loc); err == nil {
			return t, true
		}
		if t, err := time.ParseInLocation("2006-01-02", prefix, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (cs *ContentStore) SaveArticle(section, slug, rawContent string) error {
	filePath, err := cs.safeArticlePath(section, slug)
	if err != nil {
		return err
	}

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

func (cs *ContentStore) DeleteArticle(section, slug string) error {
	filePath, err := cs.safeArticlePath(section, slug)
	if err != nil {
		return err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("article not found: %s/%s", section, slug)
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("target is a directory: %s/%s", section, slug)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to remove article file: %w", err)
	}

	return nil
}

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

func parseFrontmatter(raw string) (map[string]string, string) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	fields := make(map[string]string)

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fields, raw
	}

	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return fields, raw
	}

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

	body := strings.Join(lines[closeIdx+1:], "\n")
	body = strings.TrimLeft(body, "\n")
	return fields, body
}

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
