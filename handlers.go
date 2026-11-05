package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"goseq/web"
)

type Server struct {
	cfg       *Config
	store     *ContentStore
	auth      *AuthManager
	templates *web.Templates
	mux       *http.ServeMux
}

func NewServer(cfg *Config, store *ContentStore, auth *AuthManager, tmpls *web.Templates) *Server {
	s := &Server{
		cfg:       cfg,
		store:     store,
		auth:      auth,
		templates: tmpls,
		mux:       http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	fileServer := http.FileServer(http.FS(web.FS))
	s.mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		fileServer.ServeHTTP(w, r)
	})
	s.mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFileFS(w, r, web.FS, "static/img/logo.png")
	})

	s.mux.HandleFunc("GET /{$}", s.handleIndex)
	s.mux.HandleFunc("GET /category/{slug}", s.handleCategoryList)
	s.mux.HandleFunc("GET /read/{category}/{filename}", s.handleArticleDetail)
	s.mux.HandleFunc("GET /main-site", s.handleMainSite)
	s.mux.HandleFunc("GET /uploads/{path...}", s.handleServeUpload)

	s.mux.HandleFunc("GET /duaempatdua", s.handleLoginPage)
	s.mux.HandleFunc("POST /duaempatdua", s.handleLoginSubmit)
	s.mux.HandleFunc("POST /logout", s.handleLogout)

	s.mux.HandleFunc("GET /write", s.requireAuth(s.handleWritePage))
	s.mux.HandleFunc("POST /write", s.requireAuth(s.handleSaveArticle))
	s.mux.HandleFunc("POST /delete", s.requireAuth(s.handleDeleteArticle))
	s.mux.HandleFunc("DELETE /write", s.requireAuth(s.handleDeleteArticle))
	s.mux.HandleFunc("GET /api/article/{section}/{slug}", s.requireAuth(s.handleAPIReadArticle))
	s.mux.HandleFunc("POST /api/upload", s.requireAuth(s.handleUploadImage))
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.IsAuthenticated(r) {
			if isJSONRequest(r) {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/duaempatdua", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

type IndexViewData struct {
	Title           string
	IsAdmin         bool
	ActiveNav       string
	IsWide          bool
	Sections        []Section
	ActivityCounts  map[string]int
	ActivityHeatmap *ActivityHeatmap
}

type CategoryViewData struct {
	Title        string
	IsAdmin      bool
	ActiveNav    string
	IsWide       bool
	Section      Section
	CategorySlug string
}

type ArticleViewData struct {
	Title              string
	IsAdmin            bool
	ActiveNav          string
	IsWide             bool
	Article            *Article
	Section            string
	SectionDisplayName string
	PrevArticle        *Article
	NextArticle        *Article
}

type WritePageData struct {
	Title             string
	IsAdmin           bool
	ActiveNav         string
	IsWide            bool
	Section           string
	Slug              string
	Content           string
	Lang              string
	Category          string
	AvailableSections []string
	AvailableArticles []string
	LockedDate        string
	LockedDateISO     string
	IsNewEntry        bool
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	articlesList, _ := s.store.ListArticlesInSection("articles")
	tilList, _ := s.store.ListArticlesInSection("til")

	sections := []Section{
		{
			Name:        "articles",
			DisplayName: "Articles",
			Description: sectionDescription("articles"),
			Articles:    articlesList,
		},
		{
			Name:        "til",
			DisplayName: "Today I Learned",
			Description: sectionDescription("til"),
			Articles:    tilList,
		},
	}

	heatmap := s.store.BuildHeatmap()

	data := IndexViewData{
		Title:           "Home",
		IsAdmin:         s.auth.IsAuthenticated(r),
		ActiveNav:       "home",
		IsWide:          false,
		Sections:        sections,
		ActivityCounts:  heatmap.ActivityCounts,
		ActivityHeatmap: heatmap,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.RenderIndex(w, data); err != nil {
		log.Printf("Template render error: %v", err)
	}
}

func (s *Server) handleCategoryList(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	if !s.store.SectionExists(slug) {
		http.NotFound(w, r)
		return
	}

	articles, err := s.store.ListArticlesInSection(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	section := Section{
		Name:        slug,
		DisplayName: sectionDisplayName(slug),
		Description: sectionDescription(slug),
		Articles:    articles,
	}

	data := CategoryViewData{
		Title:        section.DisplayName,
		IsAdmin:      s.auth.IsAuthenticated(r),
		ActiveNav:    "home",
		IsWide:       false,
		Section:      section,
		CategorySlug: slug,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.RenderCategory(w, data); err != nil {
		log.Printf("Template render error: %v", err)
	}
}

func (s *Server) handleArticleDetail(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	if category == "" {
		category = r.PathValue("section")
	}
	filename := r.PathValue("filename")
	if filename == "" {
		filename = r.PathValue("slug")
	}
	slug := strings.TrimSuffix(filename, ".md")

	article, err := s.store.GetArticle(category, slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	allArticles, _ := s.store.ListArticlesInSection(category)
	var prevArticle, nextArticle *Article
	for i, a := range allArticles {
		if a.Slug == slug {
			if i > 0 {
				prevArticle = allArticles[i-1]
			}
			if i+1 < len(allArticles) {
				nextArticle = allArticles[i+1]
			}
			break
		}
	}

	data := ArticleViewData{
		Title:              article.Title,
		IsAdmin:            s.auth.IsAuthenticated(r),
		ActiveNav:          "home",
		IsWide:             false,
		Article:            article,
		Section:            category,
		SectionDisplayName: sectionDisplayName(category),
		PrevArticle:        prevArticle,
		NextArticle:        nextArticle,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.RenderArticle(w, data); err != nil {
		log.Printf("Template render error: %v", err)
	}
}

func (s *Server) handleMainSite(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(web.FS, "templates/main-site.html")
	if err != nil {
		http.Error(w, "Failed to load template", http.StatusInternalServerError)
		log.Printf("Template parse error: %v", err)
		return
	}

	data := struct {
		Title     string
		IsAdmin   bool
		ActiveNav string
		IsWide    bool
	}{
		Title:     "Permana Surya Dharma",
		IsAdmin:   s.auth.IsAuthenticated(r),
		ActiveNav: "main-site",
		IsWide:    false,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template render error: %v", err)
	}
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if s.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/write", http.StatusSeeOther)
		return
	}

	data := struct {
		Title     string
		IsAdmin   bool
		ActiveNav string
		IsWide    bool
		Error     string
	}{
		Title:     "Admin Login",
		IsAdmin:   false,
		ActiveNav: "login",
		IsWide:    false,
		Error:     "",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.RenderLogin(w, data)
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)

	renderLoginError := func(status int, msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		data := struct {
			Title     string
			IsAdmin   bool
			ActiveNav string
			IsWide    bool
			Error     string
		}{
			Title:     "Admin Login",
			IsAdmin:   false,
			ActiveNav: "login",
			IsWide:    false,
			Error:     msg,
		}
		_ = s.templates.RenderLogin(w, data)
	}

	if !s.auth.rateLimiter.Allow(clientIP) {
		renderLoginError(http.StatusTooManyRequests, "Too many failed login attempts. Please wait 5 minutes before trying again.")
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if !s.auth.Authenticate(username, password) {
		s.auth.rateLimiter.RecordFailure(clientIP)
		renderLoginError(http.StatusUnauthorized, "Invalid username or password.")
		return
	}

	s.auth.rateLimiter.Reset(clientIP)
	token := s.auth.GenerateSessionToken(username)
	s.auth.SetSessionCookie(w, r, token)
	http.Redirect(w, r, "/write", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.ClearSessionCookie(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleWritePage(w http.ResponseWriter, r *http.Request) {
	sectionNames, err := s.store.ListAllSections()
	if err != nil {
		http.Error(w, "Failed to load sections", http.StatusInternalServerError)
		return
	}

	section := strings.TrimSpace(r.URL.Query().Get("section"))
	if section == "" && len(sectionNames) > 0 {
		section = sectionNames[0]
	}

	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	if slug == "" {
		slug = s.store.TodayDate()
	}

	var content string
	var availableArticles []string
	lang := "EN"
	category := ""

	if section != "" {
		articles, _ := s.store.ListArticlesInSection(section)
		for _, a := range articles {
			availableArticles = append(availableArticles, a.Slug)
		}

		if article, err := s.store.GetArticle(section, slug); err == nil {
			content = article.RawContent
			if article.Lang != "" {
				lang = article.Lang
			}
			if article.Category != "" {
				category = article.Category
			}
		}
	}

	verifiedDate := s.store.ComputeVerifiedDate(section, slug)
	dateFull := verifiedDate.Format("Monday, 02 January 2006")
	dateISO := verifiedDate.Format("2006-01-02")

	isNewEntry := strings.TrimSpace(content) == ""
	if isNewEntry {
		content = "- "
	}

	data := WritePageData{
		Title:             "Editor",
		IsAdmin:           true,
		ActiveNav:         "write",
		IsWide:            true,
		Section:           section,
		Slug:              slug,
		Content:           content,
		Lang:              lang,
		Category:          category,
		AvailableSections: sectionNames,
		AvailableArticles: availableArticles,
		LockedDate:        dateFull,
		LockedDateISO:     dateISO,
		IsNewEntry:        isNewEntry,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.RenderWrite(w, data); err != nil {
		log.Printf("Template render error: %v", err)
	}
}

func (s *Server) generateArticleTemplate(dateStr string) string {
	return "- "
}

func isSameDate(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

var datePropRegex = regexp.MustCompile(`^(\s*-\s*Date::\s*)(.*)$`)

func (s *Server) enforceDateLock(content string, verifiedDate time.Time) string {
	loc := s.store.loc
	verifiedDateFull := verifiedDate.In(loc).Format("Monday, 02 January 2006")
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	for i, line := range lines {
		matches := datePropRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			prefix := matches[1]
			val := strings.TrimSpace(matches[2])
			parsedTime, ok := parseDateString(val, loc)
			if !ok || !isSameDate(parsedTime, verifiedDate) {
				lines[i] = prefix + verifiedDateFull
			}
		}
	}

	return strings.Join(lines, "\n")
}

type SaveArticlePayload struct {
	Section string `json:"section"`
	Slug    string `json:"slug"`
	Content string `json:"content"`
}

func (s *Server) handleSaveArticle(w http.ResponseWriter, r *http.Request) {
	var section, slug, content string

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var payload SaveArticlePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		section = payload.Section
		slug = payload.Slug
		content = payload.Content
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		section = r.FormValue("section")
		slug = r.FormValue("slug")
		content = r.FormValue("content")
	}

	section = strings.TrimSpace(section)
	slug = strings.TrimSpace(slug)

	if section == "" || slug == "" {
		if isJSONRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "section and slug are required"})
			return
		}
		http.Error(w, "section and slug are required", http.StatusBadRequest)
		return
	}

	verifiedDate := s.store.ComputeVerifiedDate(section, slug)
	sanitizedContent := s.enforceDateLock(content, verifiedDate)

	if err := s.store.SaveArticle(section, slug, sanitizedContent); err != nil {
		log.Printf("Failed to save article %s/%s: %v", section, slug, err)
		if isJSONRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "Failed to save file"})
			return
		}
		http.Error(w, "Failed to save article", http.StatusInternalServerError)
		return
	}

	nowStr := time.Now().In(s.store.loc).Format("15:04:05")

	var availableArticles []string
	if articles, err := s.store.ListArticlesInSection(section); err == nil {
		for _, a := range articles {
			availableArticles = append(availableArticles, a.Slug)
		}
	}

	if isJSONRequest(r) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"section":  section,
			"slug":     slug,
			"saved_at": nowStr,
			"date":     verifiedDate.Format("Monday, 02 January 2006"),
			"content":  sanitizedContent,
			"articles": availableArticles,
		})
		return
	}

	http.Redirect(w, r, "/write?section="+section+"&slug="+slug, http.StatusSeeOther)
}

type DeleteArticlePayload struct {
	Section string `json:"section"`
	Slug    string `json:"slug"`
}

func (s *Server) handleDeleteArticle(w http.ResponseWriter, r *http.Request) {
	var section, slug string

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var payload DeleteArticlePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		section = payload.Section
		slug = payload.Slug
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		section = r.FormValue("section")
		slug = r.FormValue("slug")
	}

	section = strings.TrimSpace(section)
	slug = strings.TrimSpace(slug)

	if section == "" || slug == "" {
		if isJSONRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "section and slug are required"})
			return
		}
		http.Error(w, "section and slug are required", http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteArticle(section, slug); err != nil {
		log.Printf("Failed to delete article %s/%s: %v", section, slug, err)
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "traversal") {
			statusCode = http.StatusBadRequest
		} else if strings.Contains(err.Error(), "not found") {
			statusCode = http.StatusNotFound
		}

		if isJSONRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(statusCode)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		http.Error(w, err.Error(), statusCode)
		return
	}

	remainingArticles, _ := s.store.ListArticlesInSection(section)
	var redirectURL string
	var remainingSlugs []string
	for _, a := range remainingArticles {
		remainingSlugs = append(remainingSlugs, a.Slug)
	}

	if len(remainingSlugs) > 0 {
		redirectURL = fmt.Sprintf("/write?section=%s&slug=%s&deleted=1", url.QueryEscape(section), url.QueryEscape(remainingSlugs[0]))
	} else {
		redirectURL = fmt.Sprintf("/write?section=%s&deleted=1", url.QueryEscape(section))
	}

	if isJSONRequest(r) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":      true,
			"section":      section,
			"deleted_slug": slug,
			"redirect":     redirectURL,
			"articles":     remainingSlugs,
		})
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (s *Server) handleAPIReadArticle(w http.ResponseWriter, r *http.Request) {
	section := r.PathValue("section")
	slug := r.PathValue("slug")

	article, err := s.store.GetArticle(section, slug)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "Article not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"section": section,
		"slug":    slug,
		"content": article.RawContent,
	})
}

func (s *Server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "File exceeds 5MB size limit or invalid form"})
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		file, _, err = r.FormFile("file")
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "No image file provided in form"})
		return
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read uploaded file"})
		return
	}
	buf = buf[:n]

	mimeType := http.DetectContentType(buf)
	isWebP := mimeType == "image/webp" || (len(buf) >= 12 && string(buf[0:4]) == "RIFF" && string(buf[8:12]) == "WEBP")
	if !isWebP {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid file format: must be a WebP image"})
		return
	}

	randBytes := make([]byte, 6)
	if _, err := rand.Read(randBytes); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to generate filename"})
		return
	}
	filename := fmt.Sprintf("%d_%s.webp", time.Now().Unix(), hex.EncodeToString(randBytes))

	uploadsDir := filepath.Join(s.cfg.ContentDir, "uploads")
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to ensure upload directory exists"})
		return
	}

	destPath := filepath.Join(uploadsDir, filename)
	outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save file"})
		return
	}
	defer outFile.Close()

	if _, err := outFile.Write(buf); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to write file header"})
		return
	}

	if _, err := io.Copy(outFile, file); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to write file body"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"url":      "/uploads/" + filename,
		"filename": filename,
	})
}

func (s *Server) handleServeUpload(w http.ResponseWriter, r *http.Request) {
	relPath := r.PathValue("path")
	if relPath == "" {
		http.NotFound(w, r)
		return
	}

	cleaned := filepath.Clean(relPath)
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/..") || strings.Contains(cleaned, `\..`) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	uploadsDir, err := filepath.Abs(filepath.Join(s.cfg.ContentDir, "uploads"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(uploadsDir, cleaned)
	if !strings.HasPrefix(fullPath, uploadsDir+string(filepath.Separator)) && fullPath != uploadsDir {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	fi, err := os.Stat(fullPath)
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, fullPath)
}

func isJSONRequest(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.Contains(r.Header.Get("Content-Type"), "application/json")
}
