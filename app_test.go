package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goseq/web"
)

// setupTestEnvironment creates a temporary content store and server for testing.
func setupTestEnvironment(t *testing.T) (*Server, *Config, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "goseq-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	cfg := &Config{
		Host:          "127.0.0.1",
		Port:          "5000",
		ContentDir:    tmpDir,
		AdminUsername: "admin",
		AdminPassword: "supersecretpassword",
		SessionSecret: []byte("test-secret-key-with-32-bytes!"),
		SecureCookie:  false,
		Location:      loc,
	}

	store, err := NewContentStore(cfg.ContentDir, cfg.Location)
	if err != nil {
		t.Fatalf("failed to create content store: %v", err)
	}

	auth := NewAuthManager(cfg)
	tmpls, err := web.NewTemplates()
	if err != nil {
		t.Fatalf("failed to create templates: %v", err)
	}

	srv := NewServer(cfg, store, auth, tmpls)
	return srv, cfg, tmpDir
}

// --- Frontmatter Tests ---

func TestFrontmatterParsing(t *testing.T) {
	raw := `---
Title: My Test Article
Date: 2026-10-27
Lang: ID
Category: Technology
---
- This is the body content
  - Nested block
`
	fm, body := parseFrontmatter(raw)

	if fm["Title"] != "My Test Article" {
		t.Errorf("expected Title 'My Test Article', got %q", fm["Title"])
	}
	if fm["Date"] != "2026-10-27" {
		t.Errorf("expected Date '2026-10-27', got %q", fm["Date"])
	}
	if fm["Lang"] != "ID" {
		t.Errorf("expected Lang 'ID', got %q", fm["Lang"])
	}
	if fm["Category"] != "Technology" {
		t.Errorf("expected Category 'Technology', got %q", fm["Category"])
	}
	if !strings.Contains(body, "This is the body content") {
		t.Errorf("expected body to contain 'This is the body content', got %q", body)
	}
	if strings.Contains(body, "---") {
		t.Errorf("body should not contain frontmatter delimiters, got %q", body)
	}
}

func TestFrontmatterDefaults(t *testing.T) {
	// No frontmatter at all
	raw := "- Some content\n  - Nested"
	fm, body := parseFrontmatter(raw)

	if len(fm) != 0 {
		t.Errorf("expected empty frontmatter fields, got %v", fm)
	}
	if body != raw {
		t.Errorf("expected body to equal raw when no frontmatter, got %q", body)
	}

	// Title derived from slug
	title := slugToTitle("golang_interfaces")
	if title != "Golang Interfaces" {
		t.Errorf("expected 'Golang Interfaces', got %q", title)
	}

	title2 := slugToTitle("belajar-go")
	if title2 != "Belajar Go" {
		t.Errorf("expected 'Belajar Go', got %q", title2)
	}
}

// --- ContentStore Tests ---

func TestContentStoreSaveAndRead(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "goseq-content-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	store, err := NewContentStore(tmpDir, loc)
	if err != nil {
		t.Fatal(err)
	}

	rawContent := "---\nTitle: Test Article\nDate: 2026-10-27\nLang: EN\n---\n- Hello world\n  - Sub bullet"
	if err := store.SaveArticle("til", "test-article", rawContent); err != nil {
		t.Fatalf("failed to save article: %v", err)
	}

	// Verify file on disk
	filePath := filepath.Join(tmpDir, "til", "test-article.md")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("expected file at %s to exist", filePath)
	}

	// Read back via GetArticle
	article, err := store.GetArticle("til", "test-article")
	if err != nil {
		t.Fatalf("failed to get article: %v", err)
	}

	if article.Title != "Test Article" {
		t.Errorf("expected title 'Test Article', got %q", article.Title)
	}
	if article.Lang != "EN" {
		t.Errorf("expected lang 'EN', got %q", article.Lang)
	}
	if article.Slug != "test-article" {
		t.Errorf("expected slug 'test-article', got %q", article.Slug)
	}
	if article.Section != "til" {
		t.Errorf("expected section 'til', got %q", article.Section)
	}
	if !strings.Contains(article.RawContent, "Hello world") {
		t.Errorf("expected RawContent to contain 'Hello world', got %q", article.RawContent)
	}
	if article.Date.Year() != 2026 || article.Date.Month() != 10 || article.Date.Day() != 27 {
		t.Errorf("expected date 2026-10-27, got %v", article.Date)
	}
}

func TestContentStoreListSections(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "goseq-list-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	store, err := NewContentStore(tmpDir, loc)
	if err != nil {
		t.Fatal(err)
	}

	// Save articles in both sections
	if err := store.SaveArticle("til", "article-one", "- TIL content 1"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveArticle("til", "article-two", "- TIL content 2"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveArticle("articles", "long-form", "---\nTitle: Long Form Article\nDate: 2026-09-01\n---\n- Body"); err != nil {
		t.Fatal(err)
	}

	sections, err := store.ListSections()
	if err != nil {
		t.Fatalf("ListSections failed: %v", err)
	}

	if len(sections) < 2 {
		t.Fatalf("expected at least 2 sections, got %d", len(sections))
	}

	// Find til section
	var tilSection *Section
	for i := range sections {
		if sections[i].Name == "til" {
			tilSection = &sections[i]
			break
		}
	}
	if tilSection == nil {
		t.Fatal("expected 'til' section to exist")
	}
	if len(tilSection.Articles) != 2 {
		t.Errorf("expected 2 articles in 'til', got %d", len(tilSection.Articles))
	}
}

// --- Parser Tests (kept from original) ---

func TestParserOutlinerAndFormatting(t *testing.T) {
	raw := `- Top block
  - Nested block 1
    - Level 2 block with **bold** and *italic* and ` + "`code`" + `
  - Nested block 2 with #golang and [[Logseq]]
- TODO First task #urgent
- DONE Second task
- Block with code:
  ` + "```go\nfunc main() {\n  println(\"hello world\")\n}\n```"

	parsed := ParseJournal("2026_11_01", "Sunday, 01 November 2026", "01 Nov 2026", raw)

	if len(parsed.Roots) != 4 {
		t.Fatalf("expected 4 root blocks, got %d", len(parsed.Roots))
	}

	root0 := parsed.Roots[0]
	if len(root0.Children) != 2 {
		t.Fatalf("expected root 0 to have 2 children, got %d", len(root0.Children))
	}
	if len(root0.Children[0].Children) != 1 {
		t.Fatalf("expected root 0 child 0 to have 1 grandchild, got %d", len(root0.Children[0].Children))
	}

	root1 := parsed.Roots[1]
	if !root1.IsTodo {
		t.Errorf("expected root 1 to be TODO")
	}
	if !strings.Contains(string(root1.HTML), `class="task-badge task-todo"`) {
		t.Errorf("expected root 1 HTML to contain task-todo badge")
	}

	root2 := parsed.Roots[2]
	if !root2.IsDone {
		t.Errorf("expected root 2 to be DONE")
	}
	if !strings.Contains(string(root2.HTML), `class="task-badge task-done"`) {
		t.Errorf("expected root 2 HTML to contain task-done badge")
	}

	foundGo := false
	foundUrgent := false
	for _, tag := range parsed.Tags {
		if tag == "golang" {
			foundGo = true
		}
		if tag == "urgent" {
			foundUrgent = true
		}
	}
	if !foundGo || !foundUrgent {
		t.Errorf("expected tags 'golang' and 'urgent', got: %v", parsed.Tags)
	}

	foundLink := false
	for _, link := range parsed.PageLinks {
		if link == "Logseq" {
			foundLink = true
		}
	}
	if !foundLink {
		t.Errorf("expected page link 'Logseq', got: %v", parsed.PageLinks)
	}

	htmlStr := string(parsed.HTMLContent)
	if !strings.Contains(htmlStr, `<pre class="code-fence"><code class="language-go">`) {
		t.Errorf("expected rendered HTML to contain code fence with language-go")
	}
}

func TestParserXSSPrevention(t *testing.T) {
	raw := `- Safe text <script>alert('xss')</script> and <img src=x onerror=alert(1)>`
	parsed := ParseJournal("2026_11_01", "2026_11_01", "01 Nov", raw)

	htmlStr := string(parsed.HTMLContent)
	if strings.Contains(htmlStr, "<script>") {
		t.Errorf("raw <script> tag was not escaped!")
	}
	if !strings.Contains(htmlStr, "&lt;script&gt;") {
		t.Errorf("expected &lt;script&gt;, got: %s", htmlStr)
	}
}

// --- Auth Tests (kept from original) ---

func TestAuthManagerSessionAndRateLimit(t *testing.T) {
	cfg := &Config{
		AdminUsername: "admin",
		AdminPassword: "password123",
		SessionSecret: []byte("secret-key-32-bytes-long-here!"),
	}
	auth := NewAuthManager(cfg)

	if !auth.Authenticate("admin", "password123") {
		t.Errorf("valid credentials rejected")
	}
	if auth.Authenticate("admin", "wrongpassword") {
		t.Errorf("wrong credentials accepted")
	}
	if auth.Authenticate("wronguser", "password123") {
		t.Errorf("wrong username accepted")
	}

	token := auth.GenerateSessionToken("admin")
	if !auth.ValidateSessionToken(token) {
		t.Errorf("valid session token rejected")
	}
	if auth.ValidateSessionToken("tampered" + token) {
		t.Errorf("tampered session token accepted")
	}

	ip := "192.168.1.100"
	for i := 0; i < 4; i++ {
		if !auth.rateLimiter.Allow(ip) {
			t.Fatalf("rate limiter triggered too early at attempt %d", i)
		}
		auth.rateLimiter.RecordFailure(ip)
	}
	if !auth.rateLimiter.Allow(ip) {
		t.Fatalf("5th attempt should still be allowed")
	}
	auth.rateLimiter.RecordFailure(ip)

	if auth.rateLimiter.Allow(ip) {
		t.Errorf("rate limiter should block after 5 failed attempts")
	}

	auth.rateLimiter.Reset(ip)
	if !auth.rateLimiter.Allow(ip) {
		t.Errorf("rate limiter should allow after reset")
	}
}

// --- HTTP Route Tests ---

func TestHTTPRoutes(t *testing.T) {
	srv, _, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	// Seed a TIL article
	if err := srv.store.SaveArticle("til", "golang-interfaces", "---\nTitle: Golang Interfaces\nDate: 2026-11-01\nLang: EN\n---\n- Interface basics in Go\n  - Implicit implementation"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}

	// 1. GET / (Tier 1: 2 Prominent Cards for Articles & TIL)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / returned status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Articles") || !strings.Contains(body, "Today I Learned") {
		t.Errorf("GET / response did not contain category cards: %s", body)
	}
	if !strings.Contains(body, "/category/articles") || !strings.Contains(body, "/category/til") {
		t.Errorf("GET / response did not contain category links: %s", body)
	}

	// 1b. Verify that public pages do not leak the admin login in navbar
	if strings.Contains(body, ">Admin<") || strings.Contains(body, "/duaempatdua") || strings.Contains(body, "/login") {
		t.Errorf("public navbar contains leaked admin login links: %s", body)
	}

	// 2. GET /category/til (Tier 2: Category list view)
	req = httptest.NewRequest("GET", "/category/til", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /category/til returned status %d", rec.Code)
	}
	catBody := rec.Body.String()
	if !strings.Contains(catBody, "Golang Interfaces") {
		t.Errorf("GET /category/til did not contain article title: %s", catBody)
	}
	if !strings.Contains(catBody, "/read/til/golang-interfaces") {
		t.Errorf("GET /category/til did not contain link to article: %s", catBody)
	}

	// 2b. GET /category/nonexistent (Tier 2: 404 for unknown category)
	req = httptest.NewRequest("GET", "/category/nonexistent", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent category, got %d", rec.Code)
	}

	// 3. GET /read/til/golang-interfaces (Tier 3: Article detail view)
	req = httptest.NewRequest("GET", "/read/til/golang-interfaces", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /read/til/golang-interfaces returned status %d", rec.Code)
	}
	artBody := rec.Body.String()
	if !strings.Contains(artBody, "Interface basics in Go") {
		t.Errorf("article detail page did not contain article content")
	}
	if !strings.Contains(artBody, "/category/til") {
		t.Errorf("article detail page did not contain back link to category")
	}

	// 4. GET /main-site (Placeholder route)
	req = httptest.NewRequest("GET", "/main-site", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /main-site returned status %d", rec.Code)
	}

	// 5. GET /login should return 404 Not Found (hidden route)
	req = httptest.NewRequest("GET", "/login", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for /login, got %d", rec.Code)
	}

	// 6. GET /write (Unauthenticated → Redirect to /duaempatdua)
	req = httptest.NewRequest("GET", "/write", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unauthenticated GET /write returned status %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/duaempatdua" {
		t.Errorf("expected redirect to /duaempatdua, got %s", loc)
	}

	// 7. GET /duaempatdua (Hidden login page accessible)
	req = httptest.NewRequest("GET", "/duaempatdua", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /duaempatdua returned status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `action="/duaempatdua"`) {
		t.Errorf("login form does not post to /duaempatdua: %s", rec.Body.String())
	}

	// 8. POST /duaempatdua (Valid credentials)
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "supersecretpassword")
	req = httptest.NewRequest("POST", "/duaempatdua", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("valid login returned status %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/write" {
		t.Errorf("expected redirect to /write, got %s", loc)
	}

	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("session cookie not set on successful login")
	}

	// 5. GET /write with session cookie (Authenticated)
	req = httptest.NewRequest("GET", "/write?section=til&slug=golang-interfaces", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET /write returned status %d", rec.Code)
	}

	// 6. POST /write AJAX JSON Save (Authenticated)
	saveBody, _ := json.Marshal(map[string]string{
		"section": "til",
		"slug":    "new-article",
		"content": "- New article content\n  - Sub-bullet",
	})
	req = httptest.NewRequest("POST", "/write", bytes.NewReader(saveBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /write AJAX returned status %d: %s", rec.Code, rec.Body.String())
	}

	var jsonRes map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &jsonRes); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if jsonRes["success"] != true {
		t.Errorf("expected success: true, got %v", jsonRes)
	}

	// Verify file was saved
	savedArticle, err := srv.store.GetArticle("til", "new-article")
	if err != nil || !strings.Contains(savedArticle.RawContent, "New article content") {
		t.Errorf("saved content mismatch: err=%v content=%s", err, savedArticle.RawContent)
	}

	// 7. POST /logout
	req = httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /logout returned status %d", rec.Code)
	}
	logoutCookie := rec.Result().Cookies()[0]
	if logoutCookie.MaxAge != -1 {
		t.Errorf("logout cookie MaxAge not -1: %d", logoutCookie.MaxAge)
	}
}
