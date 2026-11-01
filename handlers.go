package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"goseq/web"
)

const PageSize = 5

type Server struct {
	cfg       *Config
	storage   *Storage
	auth      *AuthManager
	templates *web.Templates
	mux       *http.ServeMux
}

func NewServer(cfg *Config, storage *Storage, auth *AuthManager, tmpls *web.Templates) *Server {
	s := &Server{
		cfg:       cfg,
		storage:   storage,
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
	s.mux.HandleFunc("GET /journal/{date}", s.handleSingleJournal)

	// Admin auth routes
	s.mux.HandleFunc("GET /login", s.handleLoginPage)
	s.mux.HandleFunc("POST /login", s.handleLoginSubmit)
	s.mux.HandleFunc("POST /logout", s.handleLogout)

	// Admin protected routes
	s.mux.HandleFunc("GET /write", s.requireAuth(s.handleWritePage))
	s.mux.HandleFunc("POST /write", s.requireAuth(s.handleSaveJournal))
	s.mux.HandleFunc("GET /api/journal/{date}", s.requireAuth(s.handleAPIReadJournal))
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.IsAuthenticated(r) {
			if isJSONRequest(r) {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

type IndexViewData struct {
	Title       string
	TodayDate   string
	IsAdmin     bool
	ActiveNav   string
	IsWide      bool
	Entries     []*ParsedJournal
	Query       string
	ActiveTag   string
	ActiveLink  string
	CurrentPage int
	TotalPages  int
	HasPrev     bool
	HasNext     bool
	PrevPage    int
	NextPage    int
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	allDates, err := s.storage.ListAllDates()
	if err != nil {
		http.Error(w, "Failed to load journals", http.StatusInternalServerError)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	tagFilter := strings.TrimSpace(r.URL.Query().Get("tag"))
	linkFilter := strings.TrimSpace(r.URL.Query().Get("link"))

	var matchedDates []string
	cacheContent := make(map[string]string)

	for _, date := range allDates {
		raw, err := s.storage.ReadJournal(date)
		if err != nil {
			continue
		}
		cacheContent[date] = raw

		// If query is specified
		if query != "" {
			if !strings.Contains(strings.ToLower(raw), strings.ToLower(query)) &&
				!strings.Contains(date, query) {
				continue
			}
		}

		// If tag filter specified
		if tagFilter != "" {
			tagMulti := fmt.Sprintf("#[[%s]]", tagFilter)
			tagSingle := fmt.Sprintf("#%s", tagFilter)
			if !strings.Contains(raw, tagSingle) && !strings.Contains(raw, tagMulti) {
				continue
			}
		}

		// If wikilink filter specified
		if linkFilter != "" {
			wikiLink := fmt.Sprintf("[[%s]]", linkFilter)
			if !strings.Contains(raw, wikiLink) {
				continue
			}
		}

		matchedDates = append(matchedDates, date)
	}

	// Pagination
	page := 1
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}

	totalEntries := len(matchedDates)
	totalPages := int(math.Ceil(float64(totalEntries) / float64(PageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * PageSize
	endIndex := startIndex + PageSize
	if startIndex > totalEntries {
		startIndex = totalEntries
	}
	if endIndex > totalEntries {
		endIndex = totalEntries
	}

	var parsedEntries []*ParsedJournal
	for _, date := range matchedDates[startIndex:endIndex] {
		raw := cacheContent[date]
		disp := s.storage.FormatDateDisplay(date)
		short := s.storage.FormatShortDateDisplay(date)
		parsed := ParseJournal(date, disp, short, raw)
		parsedEntries = append(parsedEntries, parsed)
	}

	data := IndexViewData{
		Title:       "Journals",
		TodayDate:   s.storage.TodayDate(),
		IsAdmin:     s.auth.IsAuthenticated(r),
		ActiveNav:   "journals",
		IsWide:      false,
		Entries:     parsedEntries,
		Query:       query,
		ActiveTag:   tagFilter,
		ActiveLink:  linkFilter,
		CurrentPage: page,
		TotalPages:  totalPages,
		HasPrev:     page > 1,
		HasNext:     page < totalPages,
		PrevPage:    page - 1,
		NextPage:    page + 1,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.RenderIndex(w, data); err != nil {
		log.Printf("Template render error: %v", err)
	}
}

type JournalSingleViewData struct {
	Title         string
	TodayDate     string
	IsAdmin       bool
	ActiveNav     string
	IsWide        bool
	Entry         *ParsedJournal
	PrevDate      string
	PrevDateShort string
	NextDate      string
	NextDateShort string
}

func (s *Server) handleSingleJournal(w http.ResponseWriter, r *http.Request) {
	dateStr := r.PathValue("date")
	if !s.storage.ValidateDate(dateStr) {
		http.NotFound(w, r)
		return
	}

	raw, err := s.storage.ReadJournal(dateStr)
	if err != nil {
		http.Error(w, "Error reading journal", http.StatusInternalServerError)
		return
	}
	if raw == "" {
		exists, _ := s.storage.JournalExists(dateStr)
		if !exists {
			http.NotFound(w, r)
			return
		}
	}

	disp := s.storage.FormatDateDisplay(dateStr)
	short := s.storage.FormatShortDateDisplay(dateStr)
	parsed := ParseJournal(dateStr, disp, short, raw)

	// Find prev/next date
	allDates, _ := s.storage.ListAllDates()
	var prevDate, prevDateShort, nextDate, nextDateShort string
	for i, d := range allDates {
		if d == dateStr {
			if i > 0 {
				prevDate = allDates[i-1]
				prevDateShort = s.storage.FormatShortDateDisplay(prevDate)
			}
			if i+1 < len(allDates) {
				nextDate = allDates[i+1]
				nextDateShort = s.storage.FormatShortDateDisplay(nextDate)
			}
			break
		}
	}

	data := struct {
		Title         string
		TodayDate     string
		IsAdmin       bool
		ActiveNav     string
		IsWide        bool
		Entry         *ParsedJournal
		PrevDate      string
		PrevDateShort string
		NextDate      string
		NextDateShort string
	}{
		Title:         disp,
		TodayDate:     s.storage.TodayDate(),
		IsAdmin:       s.auth.IsAuthenticated(r),
		ActiveNav:     "journals",
		IsWide:        false,
		Entry:         parsed,
		PrevDate:      prevDate,
		PrevDateShort: prevDateShort,
		NextDate:      nextDate,
		NextDateShort: nextDateShort,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.RenderJournal(w, data); err != nil {
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
		TodayDate string
		IsAdmin   bool
		ActiveNav string
		IsWide    bool
		Error     string
	}{
		Title:     "Admin Login",
		TodayDate: s.storage.TodayDate(),
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

	if !s.auth.rateLimiter.Allow(clientIP) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusTooManyRequests)
		data := struct {
			Title     string
			TodayDate string
			IsAdmin   bool
			ActiveNav string
			IsWide    bool
			Error     string
		}{
			Title:     "Login Rate Limited",
			TodayDate: s.storage.TodayDate(),
			IsAdmin:   false,
			ActiveNav: "login",
			IsWide:    false,
			Error:     "Too many failed login attempts. Please wait 5 minutes before trying again.",
		}
		_ = s.templates.RenderLogin(w, data)
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		data := struct {
			Title     string
			TodayDate string
			IsAdmin   bool
			ActiveNav string
			IsWide    bool
			Error     string
		}{
			Title:     "Admin Login",
			TodayDate: s.storage.TodayDate(),
			IsAdmin:   false,
			ActiveNav: "login",
			IsWide:    false,
			Error:     "Invalid username or password.",
		}
		_ = s.templates.RenderLogin(w, data)
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

type WritePageData struct {
	Title          string
	TodayDate      string
	IsAdmin        bool
	ActiveNav      string
	IsWide         bool
	Date           string
	Content        string
	AvailableDates []string
}

func (s *Server) handleWritePage(w http.ResponseWriter, r *http.Request) {
	targetDate := strings.TrimSpace(r.URL.Query().Get("date"))
	if targetDate == "" {
		targetDate = s.storage.TodayDate()
	}

	if !s.storage.ValidateDate(targetDate) {
		http.Error(w, "Invalid date format (must be YYYY_MM_DD)", http.StatusBadRequest)
		return
	}

	content, err := s.storage.ReadJournal(targetDate)
	if err != nil {
		http.Error(w, "Failed to read journal", http.StatusInternalServerError)
		return
	}

	availableDates, _ := s.storage.ListAllDates()

	data := WritePageData{
		Title:          fmt.Sprintf("Write: %s", targetDate),
		TodayDate:      s.storage.TodayDate(),
		IsAdmin:        true,
		ActiveNav:      "write",
		IsWide:         true,
		Date:           targetDate,
		Content:        content,
		AvailableDates: availableDates,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.RenderWrite(w, data); err != nil {
		log.Printf("Template render error: %v", err)
	}
}

type SavePayload struct {
	Date    string `json:"date"`
	Content string `json:"content"`
}

func (s *Server) handleSaveJournal(w http.ResponseWriter, r *http.Request) {
	var date string
	var content string

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var payload SavePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		date = payload.Date
		content = payload.Content
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		date = r.FormValue("date")
		content = r.FormValue("content")
	}

	date = strings.TrimSpace(date)
	if !s.storage.ValidateDate(date) {
		if isJSONRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "Invalid date format"})
			return
		}
		http.Error(w, "Invalid date format (must be YYYY_MM_DD)", http.StatusBadRequest)
		return
	}

	if err := s.storage.SaveJournal(date, content); err != nil {
		log.Printf("Failed to save journal %s: %v", date, err)
		if isJSONRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "Failed to save file"})
			return
		}
		http.Error(w, "Failed to save journal file", http.StatusInternalServerError)
		return
	}

	nowStr := time.Now().In(s.storage.loc).Format("15:04:05")

	if isJSONRequest(r) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"date":     date,
			"saved_at": nowStr,
		})
		return
	}

	http.Redirect(w, r, "/write?date="+date, http.StatusSeeOther)
}

func (s *Server) handleAPIReadJournal(w http.ResponseWriter, r *http.Request) {
	dateStr := r.PathValue("date")
	if !s.storage.ValidateDate(dateStr) {
		http.Error(w, `{"error": "Invalid date format"}`, http.StatusBadRequest)
		return
	}

	content, err := s.storage.ReadJournal(dateStr)
	if err != nil {
		http.Error(w, `{"error": "Failed to read journal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"date":    dateStr,
		"content": content,
	})
}

func isJSONRequest(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.Contains(r.Header.Get("Content-Type"), "application/json")
}
