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
		JournalsDir:   tmpDir,
		AdminUsername: "admin",
		AdminPassword: "supersecretpassword",
		SessionSecret: []byte("test-secret-key-with-32-bytes!"),
		SecureCookie:  false,
		Location:      loc,
	}

	storage, err := NewStorage(cfg.JournalsDir, cfg.Location)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	auth := NewAuthManager(cfg)
	tmpls, err := web.NewTemplates()
	if err != nil {
		t.Fatalf("failed to create templates: %v", err)
	}

	srv := NewServer(cfg, storage, auth, tmpls)
	return srv, cfg, tmpDir
}

func TestLoadEnvFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", ".env-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	content := `
# Comment line
HOST=0.0.0.0
PORT=8080
ADMIN_USERNAME="john_doe"
ADMIN_PASSWORD='mysecretpassword'
SESSION_SECRET=some-secret-string
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Clear test env vars before loading
	os.Unsetenv("TEST_ENV_VAR")

	if err := loadEnvFile(tmpFile.Name()); err != nil {
		t.Fatalf("loadEnvFile failed: %v", err)
	}

	if os.Getenv("HOST") != "0.0.0.0" {
		t.Errorf("expected HOST 0.0.0.0, got %s", os.Getenv("HOST"))
	}
	if os.Getenv("ADMIN_USERNAME") != "john_doe" {
		t.Errorf("expected ADMIN_USERNAME john_doe, got %s", os.Getenv("ADMIN_USERNAME"))
	}
	if os.Getenv("ADMIN_PASSWORD") != "mysecretpassword" {
		t.Errorf("expected ADMIN_PASSWORD mysecretpassword, got %s", os.Getenv("ADMIN_PASSWORD"))
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

	// Verify root 0 children
	root0 := parsed.Roots[0]
	if len(root0.Children) != 2 {
		t.Fatalf("expected root 0 to have 2 children, got %d", len(root0.Children))
	}
	if len(root0.Children[0].Children) != 1 {
		t.Fatalf("expected root 0 child 0 to have 1 grandchild, got %d", len(root0.Children[0].Children))
	}

	// Verify TODO and DONE task markers
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

	// Verify tags extraction
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

	// Verify page links extraction
	foundLink := false
	for _, link := range parsed.PageLinks {
		if link == "Logseq" {
			foundLink = true
		}
	}
	if !foundLink {
		t.Errorf("expected page link 'Logseq', got: %v", parsed.PageLinks)
	}

	// Verify code fence rendering
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

func TestStorageValidationAndAtomicSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "goseq-storage-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	storage, err := NewStorage(tmpDir, loc)
	if err != nil {
		t.Fatal(err)
	}

	// Date validation
	if !storage.ValidateDate("2026_11_01") {
		t.Errorf("expected 2026_11_01 to be valid")
	}
	if storage.ValidateDate("2026_02_30") { // Feb 30 does not exist
		t.Errorf("expected 2026_02_30 to be invalid calendar date")
	}
	if storage.ValidateDate("../2026_11_01") {
		t.Errorf("expected path traversal in date to be invalid")
	}

	// Save and Read
	content := "- Hello world Logseq\n  - Sub bullet"
	if err := storage.SaveJournal("2026_11_01", content); err != nil {
		t.Fatalf("failed to save journal: %v", err)
	}

	readBack, err := storage.ReadJournal("2026_11_01")
	if err != nil {
		t.Fatalf("failed to read journal: %v", err)
	}
	if readBack != content {
		t.Errorf("content mismatch: got %q, want %q", readBack, content)
	}

	// Check file on disk
	filePath := filepath.Join(tmpDir, "2026_11_01.md")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("expected %s to exist on disk", filePath)
	}

	// List dates
	if err := storage.SaveJournal("2026_10_31", "- Halloween"); err != nil {
		t.Fatal(err)
	}
	dates, err := storage.ListAllDates()
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 2 || dates[0] != "2026_11_01" || dates[1] != "2026_10_31" {
		t.Errorf("expected sorted reverse-chronological dates [2026_11_01, 2026_10_31], got: %v", dates)
	}
}

func TestAuthManagerSessionAndRateLimit(t *testing.T) {
	cfg := &Config{
		AdminUsername: "admin",
		AdminPassword: "password123",
		SessionSecret: []byte("secret-key-32-bytes-long-here!"),
	}
	auth := NewAuthManager(cfg)

	// Auth check
	if !auth.Authenticate("admin", "password123") {
		t.Errorf("valid credentials rejected")
	}
	if auth.Authenticate("admin", "wrongpassword") {
		t.Errorf("wrong credentials accepted")
	}
	if auth.Authenticate("wronguser", "password123") {
		t.Errorf("wrong username accepted")
	}

	// Session token
	token := auth.GenerateSessionToken("admin")
	if !auth.ValidateSessionToken(token) {
		t.Errorf("valid session token rejected")
	}
	if auth.ValidateSessionToken("tampered" + token) {
		t.Errorf("tampered session token accepted")
	}

	// Rate limiter
	ip := "192.168.1.100"
	for i := 0; i < 4; i++ {
		if !auth.rateLimiter.Allow(ip) {
			t.Fatalf("rate limiter triggered too early at attempt %d", i)
		}
		auth.rateLimiter.RecordFailure(ip)
	}
	// 5th attempt: 4 failures recorded, still allows the 5th attempt
	if !auth.rateLimiter.Allow(ip) {
		t.Fatalf("5th attempt should still be allowed")
	}
	auth.rateLimiter.RecordFailure(ip)

	// 6th attempt: 5 failures recorded, should block!
	if auth.rateLimiter.Allow(ip) {
		t.Errorf("rate limiter should block after 5 failed attempts")
	}

	// Reset
	auth.rateLimiter.Reset(ip)
	if !auth.rateLimiter.Allow(ip) {
		t.Errorf("rate limiter should allow after reset")
	}
}

func TestHTTPRoutes(t *testing.T) {
	srv, _, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	// Populate an entry
	_ = srv.storage.SaveJournal("2026_11_01", "- Welcome to #GoSeq\n  - Nested [[WikiLink]]")

	// 1. GET / (Public)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / returned status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Welcome to") {
		t.Errorf("GET / response did not contain journal content")
	}

	// 2. GET /journal/2026_11_01 (Public)
	req = httptest.NewRequest("GET", "/journal/2026_11_01", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /journal/2026_11_01 returned status %d", rec.Code)
	}

	// 3. GET /write (Unauthenticated -> Redirect to /login)
	req = httptest.NewRequest("GET", "/write", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unauthenticated GET /write returned status %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %s", loc)
	}

	// 4. POST /login (Valid credentials)
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "supersecretpassword")
	req = httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("valid login returned status %d, want %d", rec.Code, http.StatusSeeOther)
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
	req = httptest.NewRequest("GET", "/write?date=2026_11_01", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET /write returned status %d", rec.Code)
	}

	// 6. POST /write AJAX JSON Save (Authenticated)
	saveBody, _ := json.Marshal(map[string]string{
		"date":    "2026_11_02",
		"content": "- Second day entry\n  TODO Task for tomorrow",
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

	// Verify file was saved to storage
	savedContent, err := srv.storage.ReadJournal("2026_11_02")
	if err != nil || !strings.Contains(savedContent, "Second day entry") {
		t.Errorf("saved content mismatch: %v, %s", err, savedContent)
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
