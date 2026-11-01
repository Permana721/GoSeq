# GoSeq — Ultra-Lightweight Daily Journal

**GoSeq** is a self-hosted, ultra-lightweight Daily Journal web application written in pure Go (1.22+) standard library with **zero external dependencies**. It compiles into a single, standalone static binary with embedded HTML templates, CSS, and Vanilla JavaScript.

Designed for near-zero RAM/CPU footprint, GoSeq runs smoothly on resource-constrained servers such as a Debian ARM64 SBC/home server behind an aaPanel or Nginx reverse proxy.

---

## Highlights & Features

- **Zero Third-Party Dependencies:** Pure Go standard library (`net/http`, `html/template`, `embed`, `crypto/hmac`, `crypto/sha256`, `crypto/subtle`, `sync`, `os`). `go.mod` has 0 third-party packages.
- **Strict Logseq Markdown Compatibility:**
  - Files stored as raw `./journals/YYYY_MM_DD.md` (e.g., `2026_11_01.md`).
  - Native 2-space indented outliner bullet lists (`- `), `TODO`/`DONE` markers, `#tags`, `[[Page Links]]`, and fenced code blocks.
  - Zero proprietary YAML frontmatter or metadata. Seamless copy-pasting to and from Logseq Desktop and Mobile.
- **Single Static Executable:** All templates, stylesheets, and client scripts are embedded at compile time using `//go:embed`. Deployment requires only the single binary and a `./journals/` folder.
- **Strict Asia/Jakarta Timezone:** Embedded `time/tzdata` strictly locked to `Asia/Jakarta` (WIB / UTC+7).
- **Public Read-Only UI:** Reverse-chronological daily journals, pagination, tag & wikilink badges, server-side search (`?q=`, `?tag=`, `?link=`), and visual indentation guide lines.
- **Logseq-Style Outliner Editor (`/write`):**
  - **Smart Bullets:** Pressing `Enter` automatically continues bullets at the current indentation level or unindents/exits if the bullet is empty.
  - **Indentation:** `Tab` indents 2 spaces; `Shift + Tab` outdents 2 spaces.
  - **Instant Save (`Ctrl+S` / `Cmd+S`):** Intercepted via JavaScript `fetch` save to `POST /write` without page reload or cursor reset, with a live status indicator (`Saved at HH:MM:SS`, `Saving...`, `Unsaved changes`).
  - **Local Draft Recovery:** Unsaved content is debounced (400ms) to `localStorage` (`journal_draft_YYYY_MM_DD`). If a local draft differs from the server file upon loading, a non-intrusive restore/discard prompt is shown. Draft is deleted upon successful save.
  - **Split & Preview:** Live real-time preview toggle and a "Copy Raw Markdown" button.
- **Secure Authentication & Rate Limiting:**
  - Single immutable admin user configured via `.env` (`ADMIN_USERNAME` and `ADMIN_PASSWORD`), verified via `crypto/subtle.ConstantTimeCompare`.
  - Stateless HMAC-SHA256 signed session cookies (`HttpOnly`, `SameSite=Lax`, configurable `Secure`).
  - In-memory IP rate limiter (`sync.Mutex`) blocking brute-force attacks after 5 failed attempts per 5 minutes.
  - Path traversal protection (strict regex `^\d{4}_\d{2}_\d{2}\.md$`) and atomic disk writes (temp file + `os.Rename`).
- **Minimalist Adaptive Theme:**
  - Adapts to system OS dark/light mode via `@media (prefers-color-scheme)`.
  - Minimalist monochrome + blue palette with centered reading layout (`max-width: 720px`).

---

## Quick Start (Development)

### 1. Prerequisites
- Go 1.22+ installed.

### 2. Configure Environment
Copy `.env.example` to `.env`:

```bash
cp .env.example .env
```

Edit `.env` to set your desired credentials and secret:

```ini
HOST=127.0.0.1
PORT=5000
JOURNALS_DIR=./journals
ADMIN_USERNAME=admin
ADMIN_PASSWORD=change_this_password
SESSION_SECRET=a_very_long_random_string_with_at_least_32_characters
SECURE_COOKIE=false
TIMEZONE=Asia/Jakarta
```

### 3. Run Locally

```bash
go run .
```

Visit [http://127.0.0.1:5000](http://127.0.0.1:5000) for the public journal view, or [http://127.0.0.1:5000/write](http://127.0.0.1:5000/write) to log in and write.

---

## Production Build & Cross-Compilation

To compile a stripped, standalone static binary with zero CGo dependencies:

### Build for Debian ARM64 (Home Server / Raspberry Pi / SBC)

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o journal-server .
```

### Build for Linux x86_64 (AMD64)

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o journal-server .
```

The resulting `journal-server` executable is ~8.8 MB with everything embedded.

---

## Deployment Behind Nginx / aaPanel

### 1. Systemd Service

Create `/etc/systemd/system/goseq.service`:

```ini
[Unit]
Description=GoSeq Daily Journal Server
After=network.target

[Service]
Type=simple
User=www-data
Group=www-data
WorkingDirectory=/var/www/goseq
ExecStart=/var/www/goseq/journal-server
Restart=always
RestartSec=5
EnvironmentFile=/var/www/goseq/.env

# Hardening
ProtectSystem=full
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now goseq
```

### 2. Nginx / aaPanel Reverse Proxy Configuration

In aaPanel or Nginx website configuration:

```nginx
server {
    listen 80;
    listen 443 ssl http2;
    server_name journal.yourdomain.com;

    # SSL certificates managed by aaPanel / Let's Encrypt
    # ssl_certificate ...
    # ssl_certificate_key ...

    location / {
        proxy_pass http://127.0.0.1:5000;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Timeouts matching Go server
        proxy_connect_timeout 10s;
        proxy_read_timeout 60s;
        proxy_send_timeout 60s;
    }
}
```

When SSL is enabled on the reverse proxy, set `SECURE_COOKIE=true` in your `.env`.

---

## Editor Keyboard Shortcuts

| Shortcut | Description |
| :--- | :--- |
| `Ctrl + S` / `Cmd + S` | Instant async save without reloading page or losing cursor position |
| `Enter` | Continues bullet at current indentation level; unindents/exits if bullet is empty |
| `Tab` | Indents current line or selection by 2 spaces |
| `Shift + Tab` | Outdents current line or selection by 2 spaces |
| `/` | Focuses the search bar from anywhere on the public journal page |

---

## Running Automated Tests

```bash
go test -v ./...
```

All tests execute in milliseconds using the Go standard library test runner.
