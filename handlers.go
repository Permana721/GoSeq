package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"goseq/web"
)

// Server is the central HTTP server struct.
type Server struct {
	cfg      *Config
	store    *ContentStore
	auth     *AuthManager
	templates *web.Templates
	mux      *http.ServeMux
}

// NewServer constructs a Server and registers all routes.
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
	// Static assets with cache headers
	fileServer := http.FileServer(http.FS(web.FS))
	s.mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		fileServer.ServeHTTP(w, r)
	})

	// Public routes
	s.mux.HandleFunc("GET /{$}", s.handleIndex)
	s.mux.HandleFunc("GET /category/{slug}", s.handleCategoryList)
	s.mux.HandleFunc("GET /read/{category}/{filename}", s.handleArticleDetail)
	s.mux.HandleFunc("GET /main-site", s.handleMainSite)

	// Auth routes
	s.mux.HandleFunc("GET /duaempatdua", s.handleLoginPage)
	s.mux.HandleFunc("POST /duaempatdua", s.handleLoginSubmit)
	s.mux.HandleFunc("POST /logout", s.handleLogout)

	// Admin protected routes
	s.mux.HandleFunc("GET /write", s.requireAuth(s.handleWritePage))
	s.mux.HandleFunc("POST /write", s.requireAuth(s.handleSaveArticle))
	s.mux.HandleFunc("GET /api/article/{section}/{slug}", s.requireAuth(s.handleAPIReadArticle))
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

// --- View Data Structs ---

// IndexViewData is passed to the home page — 2-card layout.
type IndexViewData struct {
	Title     string
	IsAdmin   bool
	ActiveNav string
	IsWide    bool
	Sections  []Section
}

// CategoryViewData is passed to the Tier 2 category list view.
type CategoryViewData struct {
	Title       string
	IsAdmin     bool
	ActiveNav   string
	IsWide      bool
	Section     Section
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
	AvailableSections []string
	AvailableArticles []string
}

// --- Handlers ---

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Tier 1: Exactly 2 prominent cards: Articles and Today I Learned (TIL)
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

	data := IndexViewData{
		Title:     "GoSeq",
		IsAdmin:   s.auth.IsAuthenticated(r),
		ActiveNav: "home",
		IsWide:    false,
		Sections:  sections,
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

	// Find prev/next within the same section
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
	data := struct {
		Title     string
		IsAdmin   bool
		ActiveNav string
		IsWide    bool
	}{
		Title:     "Main Site",
		IsAdmin:   s.auth.IsAuthenticated(r),
		ActiveNav: "main-site",
		IsWide:    false,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.RenderMainSite(w, data); err != nil {
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

	// Default to first section if available
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

	if section != "" {
		articles, _ := s.store.ListArticlesInSection(section)
		for _, a := range articles {
			availableArticles = append(availableArticles, a.Slug)
		}

		// Try to load existing article
		if article, err := s.store.GetArticle(section, slug); err == nil {
			content = article.RawContent
		}
	}

	data := WritePageData{
		Title:             "Editor",
		IsAdmin:           true,
		ActiveNav:         "write",
		IsWide:            true,
		Section:           section,
		Slug:              slug,
		Content:           content,
		AvailableSections: sectionNames,
		AvailableArticles: availableArticles,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.RenderWrite(w, data); err != nil {
		log.Printf("Template render error: %v", err)
	}
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

	if err := s.store.SaveArticle(section, slug, content); err != nil {
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

	if isJSONRequest(r) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"section":  section,
			"slug":     slug,
			"saved_at": nowStr,
		})
		return
	}

	http.Redirect(w, r, "/write?section="+section+"&slug="+slug, http.StatusSeeOther)
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

// --- Helpers ---

func isJSONRequest(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.Contains(r.Header.Get("Content-Type"), "application/json")
}
