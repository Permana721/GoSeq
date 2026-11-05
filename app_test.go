package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
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
	raw := "- Some content\n  - Nested"
	fm, body := parseFrontmatter(raw)

	if len(fm) != 0 {
		t.Errorf("expected empty frontmatter fields, got %v", fm)
	}
	if body != raw {
		t.Errorf("expected body to equal raw when no frontmatter, got %q", body)
	}

	title := slugToTitle("golang_interfaces")
	if title != "Golang Interfaces" {
		t.Errorf("expected 'Golang Interfaces', got %q", title)
	}

	title2 := slugToTitle("belajar-go")
	if title2 != "Belajar Go" {
		t.Errorf("expected 'Belajar Go', got %q", title2)
	}
}

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

	filePath := filepath.Join(tmpDir, "til", "test-article.md")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("expected file at %s to exist", filePath)
	}

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

func TestHTTPRoutes(t *testing.T) {
	srv, _, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	if err := srv.store.SaveArticle("til", "golang-interfaces", "---\nTitle: Golang Interfaces\nDate: 2026-11-01\nLang: EN\n---\n- Interface basics in Go\n  - Implicit implementation"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}

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

	if !strings.Contains(body, "card-grid") || !strings.Contains(body, "card-box") {
		t.Errorf("GET / missing card-grid or card-box: %s", body)
	}
	if !strings.Contains(body, "Thoughts, stories, and knowledge I want to share from my experiences.") {
		t.Errorf("GET / missing Articles subtext: %s", body)
	}
	if !strings.Contains(body, "Short notes on things I learned today and small daily discoveries.") {
		t.Errorf("GET / missing Today I Learned subtext: %s", body)
	}
	if !strings.Contains(body, "card-badge") || !strings.Contains(body, "→") {
		t.Errorf("GET / missing subtle badge/counter at bottom of card: %s", body)
	}
	if strings.Contains(body, "home-card-badge") {
		t.Errorf("GET / contains stray category badge: %s", body)
	}
	if strings.Contains(body, "home-hero") {
		t.Errorf("GET / contains old hero clutter: %s", body)
	}
	if !strings.Contains(body, "/static/style.css") {
		t.Errorf("GET / missing link to /static/style.css: %s", body)
	}
	if !strings.Contains(body, "/static/app.js") {
		t.Errorf("GET / missing script tag for /static/app.js: %s", body)
	}
	if !strings.Contains(body, `<link rel="icon" type="image/png" href="/static/img/logo.png">`) {
		t.Errorf("GET / missing favicon link: %s", body)
	}
	if !strings.Contains(body, `<link rel="apple-touch-icon" href="/static/img/logo.png">`) {
		t.Errorf("GET / missing apple-touch-icon link: %s", body)
	}

	favReq := httptest.NewRequest("GET", "/favicon.ico", nil)
	favRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(favRec, favReq)
	if favRec.Code != http.StatusOK {
		t.Errorf("GET /favicon.ico returned status %d, expected 200", favRec.Code)
	}
	if favRec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("GET /favicon.ico Content-Type = %s, expected image/png", favRec.Header().Get("Content-Type"))
	}

	logoReq := httptest.NewRequest("GET", "/static/img/logo.png", nil)
	logoRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(logoRec, logoReq)
	if logoRec.Code != http.StatusOK {
		t.Errorf("GET /static/img/logo.png returned status %d, expected 200", logoRec.Code)
	}

	if strings.Contains(body, ">Admin<") || strings.Contains(body, "/duaempatdua") || strings.Contains(body, "/login") {
		t.Errorf("public navbar contains leaked admin login links: %s", body)
	}
	if !strings.Contains(body, "brand-link") || !strings.Contains(body, "Permana") {
		t.Errorf("public navbar missing brand link: %s", body)
	}
	if !strings.Contains(body, ">Home<") || !strings.Contains(body, ">Main Site<") {
		t.Errorf("public navbar missing Home/Main Site links: %s", body)
	}

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

	req = httptest.NewRequest("GET", "/category/nonexistent", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent category, got %d", rec.Code)
	}

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

	req = httptest.NewRequest("GET", "/main-site", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /main-site returned status %d", rec.Code)
	}
	mainSiteBody := rec.Body.String()
	if !strings.Contains(mainSiteBody, "<!DOCTYPE html>") {
		t.Errorf("GET /main-site missing <!DOCTYPE html>")
	}
	if !strings.Contains(mainSiteBody, "tailwind.config =") {
		t.Errorf("GET /main-site missing Tailwind configuration")
	}
	if !strings.Contains(mainSiteBody, `href="/"`) || !strings.Contains(mainSiteBody, "Back to Journal") {
		t.Errorf("GET /main-site missing Back to Journal link: %s", mainSiteBody)
	}
	if strings.Contains(mainSiteBody, "site-wrapper") {
		t.Errorf("GET /main-site should not include layout.html site-wrapper")
	}

	req = httptest.NewRequest("GET", "/login", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for /login, got %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/write", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unauthenticated GET /write returned status %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/duaempatdua" {
		t.Errorf("expected redirect to /duaempatdua, got %s", loc)
	}

	req = httptest.NewRequest("GET", "/duaempatdua", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /duaempatdua returned status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `action="/duaempatdua"`) {
		t.Errorf("login form does not post to /duaempatdua: %s", rec.Body.String())
	}

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

	req = httptest.NewRequest("GET", "/write?section=til&slug=golang-interfaces", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET /write returned status %d", rec.Code)
	}

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

	savedArticle, err := srv.store.GetArticle("til", "new-article")
	if err != nil || !strings.Contains(savedArticle.RawContent, "New article content") {
		t.Errorf("saved content mismatch: err=%v content=%s", err, savedArticle.RawContent)
	}

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

	req = httptest.NewRequest("GET", "/static/style.css", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /static/style.css returned status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), ".card-grid") {
		t.Errorf("GET /static/style.css missing .card-grid styles")
	}

	req = httptest.NewRequest("GET", "/static/app.js", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /static/app.js returned status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "DOMContentLoaded") {
		t.Errorf("GET /static/app.js missing DOMContentLoaded handler")
	}
}

func TestRefactoredEditorToolbarAndCleanNote(t *testing.T) {
	srv, _, _ := setupTestEnvironment(t)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "supersecretpassword")
	req := httptest.NewRequest("POST", "/duaempatdua", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("failed to get session cookie")
	}

	req = httptest.NewRequest("GET", "/write?section=articles&slug=clean-test-note", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /write returned %d", rec.Code)
	}

	body := rec.Body.String()

	if strings.Contains(body, "editor-meta-bar") || strings.Contains(body, "Locked Date:") || strings.Contains(body, "Logseq Outliner Layout") {
		t.Errorf("locked date banner clutter still present in editor HTML: %s", body)
	}

	if strings.Contains(body, "HEADER / TITLE") || strings.Contains(body, "## CONTENT") || strings.Contains(body, "## CLOSING") {
		t.Errorf("pre-filled template clutter still present in editor HTML: %s", body)
	}

	if !strings.Contains(body, `<textarea id="editorTextarea" name="content" class="editor-textarea" placeholder="- Start writing..." autofocus spellcheck="false">- </textarea>`) {
		t.Errorf("expected clean editor defaulting to '- ', got textarea in HTML: %s", body)
	}

	if !strings.Contains(body, `id="formatToolbar"`) || (!strings.Contains(body, "formatting-toolbar") && !strings.Contains(body, "editor-toolbar")) {
		t.Errorf("formatting-toolbar not found in write interface")
	}
	toolbarActions := []string{"bold", "italic", "code", "codeblock", "wikilink", "tag", "todo", "done", "indent", "outdent"}
	for _, action := range toolbarActions {
		expectedAttr := fmt.Sprintf(`data-action="%s"`, action)
		if !strings.Contains(body, expectedAttr) {
			t.Errorf("missing formatting toolbar action button: %s", expectedAttr)
		}
	}

	if !strings.Contains(body, `title="Bold (Ctrl+B)"`) ||
		!strings.Contains(body, `title="Italic (Ctrl+I)"`) ||
		!strings.Contains(body, `title="Toggle TODO"`) ||
		!strings.Contains(body, `title="Toggle DONE"`) {
		t.Errorf("missing tooltips on toolbar buttons: %s", body)
	}

	req = httptest.NewRequest("GET", "/static/js/editor.js", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/js/editor.js returned %d", rec.Code)
	}
	jsContent := rec.Body.String()

	expectedFuncs := []string{
		"wrapSelection", "toggleLineMarker",
		"formatBold", "formatItalic", "formatCode", "formatCodeBlock",
		"formatWikiLink", "formatTag", "toggleTodoDone", "indentSelection",
	}
	for _, fn := range expectedFuncs {
		if !strings.Contains(jsContent, fn) {
			t.Errorf("editor.js missing formatting function: %s", fn)
		}
	}

	if !strings.Contains(jsContent, "setRangeText") {
		t.Errorf("editor.js missing setRangeText in wrapSelection")
	}
	if !strings.Contains(jsContent, "wrapSelection('**', '**', 'bold text')") {
		t.Errorf("editor.js missing bold wrapSelection call")
	}
	if !strings.Contains(jsContent, "wrapSelection('*', '*', 'italic text')") {
		t.Errorf("editor.js missing italic wrapSelection call")
	}
	if !strings.Contains(jsContent, "toggleLineMarker('TODO')") || !strings.Contains(jsContent, "toggleLineMarker('DONE')") {
		t.Errorf("editor.js missing toggleLineMarker calls for TODO / DONE")
	}

	if !strings.Contains(jsContent, "key === 'b'") && !strings.Contains(jsContent, "key === 'B'") {
		t.Errorf("editor.js missing Ctrl+B shortcut")
	}
	if !strings.Contains(jsContent, "key === 'i'") && !strings.Contains(jsContent, "key === 'I'") {
		t.Errorf("editor.js missing Ctrl+I shortcut")
	}
	if !strings.Contains(jsContent, "key === 'k'") && !strings.Contains(jsContent, "key === 'K'") {
		t.Errorf("editor.js missing Ctrl+K shortcut")
	}

	if !strings.Contains(jsContent, "execCommand") || !strings.Contains(jsContent, "setSelectionRange") {
		t.Errorf("editor.js missing undo history / selection range preservation")
	}

	if !strings.Contains(jsContent, "formatToolbar.querySelectorAll('.toolbar-btn')") || !strings.Contains(jsContent, "e.preventDefault()") {
		t.Errorf("editor.js missing mousedown preventDefault for toolbar buttons")
	}

	req = httptest.NewRequest("GET", "/static/style.css", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	cssContent := rec.Body.String()
	if !strings.Contains(cssContent, ".editor-toolbar") || !strings.Contains(cssContent, ".toolbar-btn") {
		t.Errorf("style.css missing .editor-toolbar or .toolbar-btn styles")
	}
	if !strings.Contains(cssContent, ".editor-textarea") {
		t.Errorf("style.css missing .editor-textarea styles")
	}
	if !strings.Contains(cssContent, ".badge-todo-btn") || !strings.Contains(cssContent, ".badge-done-btn") {
		t.Errorf("style.css missing .badge-todo-btn or .badge-done-btn styles")
	}
}

func TestLanguageToggleAndBidirectionalSync(t *testing.T) {
	srv, _, tmpDir := setupTestEnvironment(t)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "supersecretpassword")
	req := httptest.NewRequest("POST", "/duaempatdua", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("failed to get session cookie")
	}

	savePayload := map[string]string{
		"section": "articles",
		"slug":    "test-id-article",
		"content": "- lang:: ID\n- # Judul Bahasa Indonesia\n- Konten artikel bahasa indonesia",
	}
	saveBody, _ := json.Marshal(savePayload)
	req = httptest.NewRequest("POST", "/write", bytes.NewReader(saveBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /write returned %d: %s", rec.Code, rec.Body.String())
	}

	art, err := srv.store.GetArticle("articles", "test-id-article")
	if err != nil {
		t.Fatalf("failed to get article: %v", err)
	}
	if art.Lang != "ID" {
		t.Errorf("expected Lang 'ID', got %q", art.Lang)
	}
	if art.Title != "Judul Bahasa Indonesia" {
		t.Errorf("expected Title 'Judul Bahasa Indonesia', got %q", art.Title)
	}

	req = httptest.NewRequest("GET", "/category/articles", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /category/articles returned %d", rec.Code)
	}
	catHTML := rec.Body.String()
	if !strings.Contains(catHTML, `lang-badge lang-id`) || !strings.Contains(catHTML, `ID`) {
		t.Errorf("category list missing ID language badge for ID article")
	}

	req = httptest.NewRequest("GET", "/write?section=articles&slug=test-id-article", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /write returned %d", rec.Code)
	}
	writeHTML := rec.Body.String()
	if !strings.Contains(writeHTML, `<option value="ID" selected>ID</option>`) {
		t.Errorf("expected ID option to be selected in langSelect: %s", writeHTML)
	}
	if !strings.Contains(writeHTML, `id="langToggleBtn"`) || !strings.Contains(writeHTML, `lang-active-id`) || !strings.Contains(writeHTML, `>ID</button>`) {
		t.Errorf("expected langToggleBtn to show ID with lang-active-id class: %s", writeHTML)
	}

	req = httptest.NewRequest("GET", "/static/js/editor.js", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	js := rec.Body.String()

	checks := []string{
		"parseLangFromContent",
		"setLangInContent",
		"syncUIState",
		"updateLangInEditor",
		"setLanguage",
		"toggleLang",
		"syncLangFromContent",
		"triggerButtonFlash",
		"langSelect.addEventListener('change'",
		"langToggleBtn.addEventListener('click'",
		"key === 'l' || e.key === 'L'",
		"textarea.dispatchEvent(new Event('input'",
		"- lang:: ",
	}
	for _, check := range checks {
		if !strings.Contains(js, check) {
			t.Errorf("editor.js missing expected synchronization logic: %q", check)
		}
	}

	if l := extractOutlinerLang("- lang:: ID\n- content"); l != "ID" {
		t.Errorf("extractOutlinerLang failed for '- lang:: ID', got %q", l)
	}
	if l := extractOutlinerLang("lang:: ID\n- content"); l != "ID" {
		t.Errorf("extractOutlinerLang failed for 'lang:: ID', got %q", l)
	}
	if l := extractOutlinerLang("- lang:: EN\n- content"); l != "EN" {
		t.Errorf("extractOutlinerLang failed for '- lang:: EN', got %q", l)
	}
	if l := extractOutlinerLang("- Date:: 2026-11-02\n- lang:: ID\n- content"); l != "ID" {
		t.Errorf("extractOutlinerLang failed with leading Date::, got %q", l)
	}

	_ = tmpDir
}

func TestActivityHeatmapContributionGraph(t *testing.T) {
	srv, _, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	if err := srv.store.SaveArticle("articles", "art-1", "---\nTitle: Art 1\nDate: 2026-10-25\n---\n- Post 1"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}
	if err := srv.store.SaveArticle("articles", "art-2", "---\nTitle: Art 2\nDate: 2026-10-26\n---\n- Post 2"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}
	if err := srv.store.SaveArticle("til", "til-1", "---\nTitle: TIL 1\nDate: 2026-10-26\n---\n- Post 3"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}
	if err := srv.store.SaveArticle("articles", "art-3", "---\nTitle: Art 3\nDate: 2026-10-27\n---\n- Post 4"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}
	if err := srv.store.SaveArticle("til", "til-2", "---\nTitle: TIL 2\nDate: 2026-10-27\n---\n- Post 5"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}
	if err := srv.store.SaveArticle("til", "til-3", "---\nTitle: TIL 3\nDate: 2026-10-27\n---\n- Post 6"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}
	if err := srv.store.SaveArticle("articles", "art-4", "---\nTitle: Art 4\nDate: 2026-10-28\n---\n- Post 7"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}
	if err := srv.store.SaveArticle("articles", "art-5", "---\nTitle: Art 5\nDate: 2026-10-28\n---\n- Post 8"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}
	if err := srv.store.SaveArticle("til", "til-4", "---\nTitle: TIL 4\nDate: 2026-10-28\n---\n- Post 9"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}
	if err := srv.store.SaveArticle("til", "til-5", "---\nTitle: TIL 5\nDate: 2026-10-28\n---\n- Post 10"); err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}

	counts, articlesByDate := srv.store.GetActivityData()
	if counts["2026-10-25"] != 1 {
		t.Errorf("expected 1 post on 2026-10-25, got %d", counts["2026-10-25"])
	}
	if counts["2026-10-26"] != 2 {
		t.Errorf("expected 2 posts on 2026-10-26, got %d", counts["2026-10-26"])
	}
	if counts["2026-10-27"] != 3 {
		t.Errorf("expected 3 posts on 2026-10-27, got %d", counts["2026-10-27"])
	}
	if counts["2026-10-28"] != 4 {
		t.Errorf("expected 4 posts on 2026-10-28, got %d", counts["2026-10-28"])
	}
	if len(articlesByDate["2026-10-28"]) != 4 {
		t.Errorf("expected 4 articles recorded for 2026-10-28, got %d", len(articlesByDate["2026-10-28"]))
	}

	heatmap := srv.store.BuildHeatmap()
	if heatmap == nil {
		t.Fatal("BuildHeatmap returned nil")
	}
	if len(heatmap.Weeks) != 53 {
		t.Errorf("expected 53 weeks, got %d", len(heatmap.Weeks))
	}
	if heatmap.TotalContributions != 10 {
		t.Errorf("expected 10 total contributions, got %d", heatmap.TotalContributions)
	}
	if len(heatmap.Months) == 0 {
		t.Errorf("expected month labels to be populated")
	}

	found25, found26, found27, found28 := false, false, false, false
	for _, w := range heatmap.Weeks {
		for _, d := range w.Days {
			switch d.DateStr {
			case "2026-10-25":
				found25 = true
				if d.Level != 1 {
					t.Errorf("expected Level 1 for 1 post, got %d", d.Level)
				}
			case "2026-10-26":
				found26 = true
				if d.Level != 2 {
					t.Errorf("expected Level 2 for 2 posts, got %d", d.Level)
				}
			case "2026-10-27":
				found27 = true
				if d.Level != 3 {
					t.Errorf("expected Level 3 for 3 posts, got %d", d.Level)
				}
			case "2026-10-28":
				found28 = true
				if d.Level != 4 {
					t.Errorf("expected Level 4 for 4 posts, got %d", d.Level)
				}
			}
		}
	}
	if !found25 || !found26 || !found27 || !found28 {
		t.Errorf("expected all seeded dates to be present in heatmap: 25=%v 26=%v 27=%v 28=%v", found25, found26, found27, found28)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / returned status %d", rec.Code)
	}

	html := rec.Body.String()
	expectedHTMLSnippets := []string{
		`class="heatmap-card"`,
		`Activity in the last year`,
		`class="heatmap-scroll-container"`,
		`class="heatmap-months"`,
		`class="heatmap-day-labels"`,
		`<span>Mon</span>`,
		`<span>Wed</span>`,
		`<span>Fri</span>`,
		`class="heatmap-grid"`,
		`class="heatmap-cell level-0"`,
		`level-1`,
		`level-2`,
		`level-3`,
		`level-4`,
		`class="heatmap-legend"`,
		`Less`,
		`More`,
		`id="heatmapDetails"`,
	}
	for _, snip := range expectedHTMLSnippets {
		if !strings.Contains(html, snip) {
			t.Errorf("GET / HTML missing heatmap element: %q", snip)
		}
	}

	req = httptest.NewRequest("GET", "/static/style.css", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	css := rec.Body.String()

	expectedCSS := []string{
		"--heatmap-l0",
		"--heatmap-l1",
		"--heatmap-l2",
		"--heatmap-l3",
		"--heatmap-l4",
		"#161b22",
		"#ebedf0",
		"#1e3a8a",
		"#2563eb",
		"#3b82f6",
		"#60a5fa",
		"grid-auto-flow: column",
		"grid-template-rows: repeat(7, 10px)",
		".heatmap-tooltip",
		".heatmap-details",
	}
	for _, c := range expectedCSS {
		if !strings.Contains(css, c) {
			t.Errorf("style.css missing expected heatmap style: %q", c)
		}
	}

	req = httptest.NewRequest("GET", "/static/app.js", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	js := rec.Body.String()

	expectedJS := []string{
		"initHeatmap",
		"heatmapTooltip",
		"heatmapDetails",
		"scrollLeft = scrollArea.scrollWidth",
	}
	for _, j := range expectedJS {
		if !strings.Contains(js, j) {
			t.Errorf("app.js missing expected heatmap interaction logic: %q", j)
		}
	}
}

func loginTestUser(t *testing.T, srv *Server) *http.Cookie {
	t.Helper()
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "supersecretpassword")
	req := httptest.NewRequest("POST", "/duaempatdua", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			return c
		}
	}
	t.Fatalf("login failed: session cookie not found")
	return nil
}

func TestCreateAndDeleteEntryStorageLayer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "goseq-del-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	store, err := NewContentStore(tmpDir, loc)
	if err != nil {
		t.Fatal(err)
	}

	section := "articles"
	slug := "test-delete-slug"
	content := "- lang:: EN\n- category:: Testing\n- Content line"
	if err := store.SaveArticle(section, slug, content); err != nil {
		t.Fatalf("failed to save article: %v", err)
	}

	articlePath := filepath.Join(tmpDir, section, slug+".md")
	if _, err := os.Stat(articlePath); err != nil {
		t.Fatalf("expected file to exist at %s", articlePath)
	}

	article, err := store.GetArticle(section, slug)
	if err != nil {
		t.Fatalf("failed to get article: %v", err)
	}
	if article.Category != "Testing" {
		t.Errorf("expected Category 'Testing', got %q", article.Category)
	}

	if err := store.DeleteArticle(section, slug); err != nil {
		t.Fatalf("failed to delete article: %v", err)
	}

	if _, err := os.Stat(articlePath); !os.IsNotExist(err) {
		t.Fatalf("expected file at %s to be removed", articlePath)
	}

	if err := store.DeleteArticle(section, "non-existent-slug"); err == nil {
		t.Errorf("expected error deleting non-existent article, got nil")
	}
}

func TestDeleteArticlePathTraversal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "goseq-traversal-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	store, err := NewContentStore(tmpDir, loc)
	if err != nil {
		t.Fatal(err)
	}

	traversalSlugs := []string{
		"../escape",
		"../../etc/passwd",
		"foo/bar",
		"sub/../../secret",
		"",
	}

	for _, slug := range traversalSlugs {
		if err := store.DeleteArticle("articles", slug); err == nil {
			t.Errorf("expected path traversal error for slug %q, got nil", slug)
		}
	}

	traversalSections := []string{
		"../articles",
		"/etc",
		"articles/..",
		"",
	}

	for _, sec := range traversalSections {
		if err := store.DeleteArticle(sec, "safe-slug"); err == nil {
			t.Errorf("expected path traversal error for section %q, got nil", sec)
		}
	}
}

func TestCategoryAndMetadataHidingInParser(t *testing.T) {
	raw := `- lang:: ID
- category:: Rekayasa Perangkat Lunak
- topic:: Sistem Terdistribusi
- Date:: 2026-11-02
- # Top Level Title
  - Nested point 1
  - Nested point 2`

	htmlContent, _, _ := ParseContent(raw)
	rendered := string(htmlContent)

	if strings.Contains(rendered, "lang::") {
		t.Errorf("rendered HTML should not contain 'lang::', got:\n%s", rendered)
	}
	if strings.Contains(rendered, "category::") {
		t.Errorf("rendered HTML should not contain 'category::', got:\n%s", rendered)
	}
	if strings.Contains(rendered, "topic::") {
		t.Errorf("rendered HTML should not contain 'topic::', got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Rekayasa Perangkat Lunak") {
		t.Errorf("rendered HTML should hide category value 'Rekayasa Perangkat Lunak', got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Sistem Terdistribusi") {
		t.Errorf("rendered HTML should hide topic value 'Sistem Terdistribusi', got:\n%s", rendered)
	}

	if !strings.Contains(rendered, "Date::") {
		t.Errorf("rendered HTML should contain 'Date::', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Top Level Title") {
		t.Errorf("rendered HTML should contain 'Top Level Title', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Nested point 1") {
		t.Errorf("rendered HTML should contain 'Nested point 1', got:\n%s", rendered)
	}
}

func TestHTTPDeleteAndCreateFlow(t *testing.T) {
	srv, _, _ := setupTestEnvironment(t)

	authCookie := loginTestUser(t, srv)

	for _, slug := range []string{"article-1", "article-2"} {
		savePayload, _ := json.Marshal(map[string]string{
			"section": "til",
			"slug":    slug,
			"content": "- lang:: EN\n- category:: Go\n- Content for " + slug,
		})
		req := httptest.NewRequest("POST", "/write", bytes.NewReader(savePayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.AddCookie(authCookie)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("failed to create %s: code %d, body: %s", slug, rec.Code, rec.Body.String())
		}

		var res map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode json response: %v", err)
		}
		if res["success"] != true {
			t.Errorf("expected success true in save response")
		}
		articles, ok := res["articles"].([]any)
		if !ok || len(articles) == 0 {
			t.Errorf("expected available articles in save response, got %v", res["articles"])
		}
	}

	delPayload, _ := json.Marshal(map[string]string{
		"section": "til",
		"slug":    "article-1",
	})
	unauthReq := httptest.NewRequest("POST", "/delete", bytes.NewReader(delPayload))
	unauthReq.Header.Set("Content-Type", "application/json")
	unauthRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", unauthRec.Code)
	}

	authDelReq := httptest.NewRequest("POST", "/delete", bytes.NewReader(delPayload))
	authDelReq.Header.Set("Content-Type", "application/json")
	authDelReq.Header.Set("Accept", "application/json")
	authDelReq.AddCookie(authCookie)
	authDelRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(authDelRec, authDelReq)

	if authDelRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on delete, got %d: %s", authDelRec.Code, authDelRec.Body.String())
	}
	var delRes map[string]any
	if err := json.NewDecoder(authDelRec.Body).Decode(&delRes); err != nil {
		t.Fatalf("failed to decode delete response: %v", err)
	}
	if delRes["success"] != true {
		t.Errorf("expected success true in delete response")
	}
	if !strings.Contains(fmt.Sprint(delRes["redirect"]), "article-2") {
		t.Errorf("expected redirect to remaining article-2, got %v", delRes["redirect"])
	}

	del2Payload, _ := json.Marshal(map[string]string{
		"section": "til",
		"slug":    "article-2",
	})
	deleteReq := httptest.NewRequest("DELETE", "/write", bytes.NewReader(del2Payload))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteReq.Header.Set("Accept", "application/json")
	deleteReq.AddCookie(authCookie)
	deleteRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on DELETE /write, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	delNotFoundReq := httptest.NewRequest("POST", "/delete", bytes.NewReader(del2Payload))
	delNotFoundReq.Header.Set("Content-Type", "application/json")
	delNotFoundReq.Header.Set("Accept", "application/json")
	delNotFoundReq.AddCookie(authCookie)
	delNotFoundRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(delNotFoundRec, delNotFoundReq)
	if delNotFoundRec.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for non-existent article delete, got %d", delNotFoundRec.Code)
	}

	travPayload, _ := json.Marshal(map[string]string{
		"section": "til",
		"slug":    "../../evil",
	})
	travReq := httptest.NewRequest("POST", "/delete", bytes.NewReader(travPayload))
	travReq.Header.Set("Content-Type", "application/json")
	travReq.Header.Set("Accept", "application/json")
	travReq.AddCookie(authCookie)
	travRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(travRec, travReq)
	if travRec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for traversal delete, got %d", travRec.Code)
	}
}

func TestWritePageTemplateNewControls(t *testing.T) {
	srv, _, _ := setupTestEnvironment(t)
	authCookie := loginTestUser(t, srv)

	req := httptest.NewRequest("GET", "/write?section=articles&slug=new-test", nil)
	req.AddCookie(authCookie)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()

	expectedElements := []string{
		`id="newEntryBtn"`,
		`+ New`,
		`id="categoryInput"`,
		`placeholder="Topic / Category (e.g. Rekayasa Perangkat Lunak)"`,
		`id="deleteBtn"`,
		`class="btn btn-sm btn-delete"`,
		`id="articleDropdown"`,
		`Jump to article...`,
	}

	for _, elem := range expectedElements {
		if !strings.Contains(body, elem) {
			t.Errorf("write.html missing expected element: %q", elem)
		}
	}
}

func TestMainSiteIndependentStandalone(t *testing.T) {
	srv, _, _ := setupTestEnvironment(t)

	req := httptest.NewRequest("GET", "/main-site", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /main-site returned status %d, expected 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Errorf("expected standalone DOCTYPE")
	}
	if !strings.Contains(body, "tailwind.config =") {
		t.Errorf("expected custom Tailwind configuration in standalone page")
	}
	if strings.Contains(body, "site-wrapper") {
		t.Errorf("standalone main-site should not contain layout.html site-wrapper")
	}
	if !strings.Contains(body, `href="/"`) || !strings.Contains(body, "Back to Journal") {
		t.Errorf("missing back to journal navigation link in navbar")
	}
}

func TestArticleProseParser(t *testing.T) {
	raw := `# Top Level Heading
This is a standard flowing paragraph with **bold**, *italic*, and ` + "`code`" + `.
Another line in the same paragraph.

## Sub Heading
> This is a blockquote.
> Multiline quote.

- Unordered item 1
- Unordered item 2

1. Ordered step 1
2. Ordered step 2

---

` + "```go\nfunc main() {\n\tprintln(\"hello world\")\n}\n```" + `

![System Architecture](/uploads/arch.webp)

- lang:: ID
- category:: Tech

[[Go Architecture]] #system #architecture
`

	htmlContent, tags, pageLinks := ParseArticle(raw)
	rendered := string(htmlContent)

	if !strings.Contains(rendered, `<p class="prose-p">`) {
		t.Errorf("expected <p class=\"prose-p\">, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "outliner-root") || strings.Contains(rendered, "outliner-item") {
		t.Errorf("prose parser should not contain outliner-root or outliner-item, got:\n%s", rendered)
	}

	if !strings.Contains(rendered, `<h2 class="prose-h2">Top Level Heading</h2>`) {
		t.Errorf("expected <h2 class=\"prose-h2\">Top Level Heading</h2>, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, `<h2 class="prose-h2">Sub Heading</h2>`) {
		t.Errorf("expected <h2 class=\"prose-h2\">Sub Heading</h2>, got:\n%s", rendered)
	}

	if !strings.Contains(rendered, `<blockquote class="prose-quote">`) {
		t.Errorf("expected <blockquote class=\"prose-quote\">, got:\n%s", rendered)
	}

	if !strings.Contains(rendered, `<ul class="prose-ul">`) || !strings.Contains(rendered, "<li>Unordered item 1</li>") {
		t.Errorf("expected <ul class=\"prose-ul\"> with list items, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, `<ol class="prose-ol">`) || !strings.Contains(rendered, "<li>Ordered step 1</li>") {
		t.Errorf("expected <ol class=\"prose-ol\"> with list items, got:\n%s", rendered)
	}

	if !strings.Contains(rendered, `<hr class="prose-hr" />`) {
		t.Errorf("expected <hr class=\"prose-hr\" />, got:\n%s", rendered)
	}

	if !strings.Contains(rendered, `<div class="code-block-wrapper">`) {
		t.Errorf("expected <div class=\"code-block-wrapper\">, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, `<code class="language-go">`) {
		t.Errorf("expected code language class, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, `<button type="button" class="copy-code-btn"`) {
		t.Errorf("expected copy-code-btn in code block, got:\n%s", rendered)
	}

	expectedFigure := `<figure class="prose-figure"><img src="/uploads/arch.webp" alt="System Architecture" class="prose-img" loading="lazy" /><figcaption class="prose-caption">System Architecture</figcaption></figure>`
	if !strings.Contains(rendered, expectedFigure) {
		t.Errorf("expected figure with figcaption, got:\n%s", rendered)
	}

	if !strings.Contains(rendered, "wikilink-badge") || !strings.Contains(rendered, "Go Architecture") {
		t.Errorf("expected wikilink-badge for [[Go Architecture]], got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "tag-badge") || !strings.Contains(rendered, "#system") {
		t.Errorf("expected tag-badge for #system, got:\n%s", rendered)
	}

	if strings.Contains(rendered, "lang::") || strings.Contains(rendered, "category::") {
		t.Errorf("metadata properties should be hidden, got:\n%s", rendered)
	}

	if len(tags) < 2 || tags[0] != "architecture" || tags[1] != "system" {
		t.Errorf("expected extracted tags [architecture system], got: %v", tags)
	}
	if len(pageLinks) < 1 || pageLinks[0] != "Go Architecture" {
		t.Errorf("expected extracted pageLinks [Go Architecture], got: %v", pageLinks)
	}

	rt := calculateReadingTime(raw)
	if rt == "" || !strings.HasSuffix(rt, "min read") {
		t.Errorf("expected valid reading time, got %q", rt)
	}
}

func TestPreserveOutlinerModeForNonArticles(t *testing.T) {
	srv, cfg, _ := setupTestEnvironment(t)

	tilContent := `---
Title: TIL Note
Date: 2026-11-01
---
This is a standard flowing prose paragraph in TIL.

- First bullet item
- Second bullet item
`
	err := srv.store.SaveArticle("til", "til-note", tilContent)
	if err != nil {
		t.Fatalf("failed to save TIL note: %v", err)
	}

	tilArticle, err := srv.store.GetArticle("til", "til-note")
	if err != nil {
		t.Fatalf("failed to get TIL note: %v", err)
	}
	tilHTML := string(tilArticle.HTMLContent)
	if !strings.Contains(tilHTML, "prose-p") {
		t.Errorf("TIL notes must use prose layout, got:\n%s", tilHTML)
	}
	if strings.Contains(tilHTML, "outliner-root") {
		t.Errorf("TIL notes should not render outliner-root, got:\n%s", tilHTML)
	}
	if tilArticle.ReadingTime == "" {
		t.Errorf("expected ReadingTime to be populated on TIL notes")
	}

	reqTIL := httptest.NewRequest("GET", "/read/til/til-note", nil)
	recTIL := httptest.NewRecorder()
	srv.mux.ServeHTTP(recTIL, reqTIL)
	if recTIL.Code != http.StatusOK {
		t.Fatalf("GET /read/til/til-note returned %d", recTIL.Code)
	}
	tilBody := recTIL.Body.String()
	if !strings.Contains(tilBody, "article-prose") {
		t.Errorf("TIL detail page missing 'article-prose' class: %s", tilBody)
	}
	if !strings.Contains(tilBody, "article-reading-time") {
		t.Errorf("TIL detail page missing 'article-reading-time' element: %s", tilBody)
	}

	if err := os.MkdirAll(filepath.Join(cfg.ContentDir, "journals"), 0755); err != nil {
		t.Fatalf("failed to create journals dir: %v", err)
	}
	journalContent := `---
Title: Journal Note
Date: 2026-11-01
---
- First bullet item
  - Child bullet item
- Second bullet item
`
	err = srv.store.SaveArticle("journals", "journal-note", journalContent)
	if err != nil {
		t.Fatalf("failed to save journal note: %v", err)
	}

	journalArticle, err := srv.store.GetArticle("journals", "journal-note")
	if err != nil {
		t.Fatalf("failed to get journal note: %v", err)
	}
	journalHTML := string(journalArticle.HTMLContent)
	if !strings.Contains(journalHTML, "outliner-root") || !strings.Contains(journalHTML, "bullet") {
		t.Errorf("journals must preserve outliner mode, got:\n%s", journalHTML)
	}
	if strings.Contains(journalHTML, "prose-p") {
		t.Errorf("journals should not use prose-p layout, got:\n%s", journalHTML)
	}

	articleContent := `---
Title: Modern Article
Date: 2026-11-01
---
This is a standard flowing prose paragraph.

Another paragraph here.
`
	err = srv.store.SaveArticle("articles", "modern-article", articleContent)
	if err != nil {
		t.Fatalf("failed to save article: %v", err)
	}

	art, err := srv.store.GetArticle("articles", "modern-article")
	if err != nil {
		t.Fatalf("failed to get article: %v", err)
	}
	artHTML := string(art.HTMLContent)
	if !strings.Contains(artHTML, "prose-p") {
		t.Errorf("articles must use prose layout, got:\n%s", artHTML)
	}
	if strings.Contains(artHTML, "outliner-root") {
		t.Errorf("articles should not render outliner-root, got:\n%s", artHTML)
	}
	if art.ReadingTime == "" {
		t.Errorf("expected ReadingTime to be populated on articles")
	}

	req := httptest.NewRequest("GET", "/read/articles/modern-article", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /read/articles/modern-article returned %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "article-prose") {
		t.Errorf("article detail page missing 'article-prose' class: %s", body)
	}
	if !strings.Contains(body, "article-reading-time") {
		t.Errorf("article detail page missing 'article-reading-time' element: %s", body)
	}
}

func TestImageUploadEndpoint(t *testing.T) {
	srv, _, tmpDir := setupTestEnvironment(t)
	authCookie := loginTestUser(t, srv)

	validWebP := []byte("RIFF\x14\x00\x00\x00WEBPVP8 \x08\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")

	{
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("image", "test.webp")
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		part.Write(validWebP)
		writer.Close()

		req := httptest.NewRequest("POST", "/api/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for unauth upload, got %d", rec.Code)
		}
	}

	{
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("image", "test.txt")
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		part.Write([]byte("not an image at all"))
		writer.Close()

		req := httptest.NewRequest("POST", "/api/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		req.AddCookie(authCookie)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for non-webp upload, got %d", rec.Code)
		}
	}

	var uploadedURL string
	var uploadedFilename string
	{
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("image", "screenshot.webp")
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		part.Write(validWebP)
		writer.Close()

		req := httptest.NewRequest("POST", "/api/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		req.AddCookie(authCookie)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for valid WebP upload, got %d: %s", rec.Code, rec.Body.String())
		}

		var res map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to parse json response: %v", err)
		}

		uploadedURL = res["url"]
		uploadedFilename = res["filename"]

		if !strings.HasPrefix(uploadedURL, "/uploads/") || !strings.HasSuffix(uploadedURL, ".webp") {
			t.Errorf("expected url starting with /uploads/ and ending with .webp, got %q", uploadedURL)
		}

		filePath := filepath.Join(tmpDir, "uploads", uploadedFilename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("uploaded file not found on disk at %s: %v", filePath, err)
		}
		if !bytes.Equal(data, validWebP) {
			t.Errorf("uploaded file content mismatch")
		}
	}

	{
		req := httptest.NewRequest("GET", uploadedURL, nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK serving uploaded image, got %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Cache-Control"), "public") {
			t.Errorf("expected Cache-Control header on served upload, got %q", rec.Header().Get("Cache-Control"))
		}
		if !bytes.Equal(rec.Body.Bytes(), validWebP) {
			t.Errorf("served image content mismatch")
		}
	}

	{
		req := httptest.NewRequest("GET", "/uploads/dummy", nil)
		req.SetPathValue("path", "../secret.txt")
		rec := httptest.NewRecorder()
		srv.handleServeUpload(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for path traversal attempt, got %d", rec.Code)
		}
	}

	{
		req := httptest.NewRequest("GET", "/uploads/non_existent.webp", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for missing upload, got %d", rec.Code)
		}
	}
}

func TestEditorClientScriptsAndStyles(t *testing.T) {
	srv, _, _ := setupTestEnvironment(t)
	authCookie := loginTestUser(t, srv)

	req := httptest.NewRequest("GET", "/static/js/editor.js", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/js/editor.js returned %d", rec.Code)
	}
	editorJS := rec.Body.String()
	requiredJSStrings := []string{
		"compressAndUploadImage",
		"uploadWebPImageBlob",
		"image/webp",
		"0.8",
		"insertTextAtCursor",
		"drag-over",
		"renderClientArticleProse",
	}
	for _, s := range requiredJSStrings {
		if !strings.Contains(editorJS, s) {
			t.Errorf("editor.js missing expected script logic: %q", s)
		}
	}

	req = httptest.NewRequest("GET", "/write?section=articles&slug=new-test", nil)
	req.AddCookie(authCookie)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /write returned %d", rec.Code)
	}
	writeHTML := rec.Body.String()
	if !strings.Contains(writeHTML, `id="imageUploadBtn"`) {
		t.Errorf("write.html missing imageUploadBtn")
	}
	if !strings.Contains(writeHTML, `id="imageFileInput"`) {
		t.Errorf("write.html missing imageFileInput")
	}

	req = httptest.NewRequest("GET", "/static/style.css", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/style.css returned %d", rec.Code)
	}
	styleCSS := rec.Body.String()
	requiredCSSStrings := []string{
		".article-prose",
		".prose-p",
		".prose-h2",
		".prose-figure",
		".prose-img",
		".copy-code-btn",
		".editor-box.drag-over",
	}
	for _, s := range requiredCSSStrings {
		if !strings.Contains(styleCSS, s) {
			t.Errorf("style.css missing expected class: %q", s)
		}
	}
}
