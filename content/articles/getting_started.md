---
Title: Getting Started with GoSeq
Date: 2026-10-28
Lang: EN
Category: Meta
---
- GoSeq is a minimal blog / digital garden engine written in Go
  - Flat file storage — every article is a plain `.md` file
  - Outliner-style content model inspired by Logseq
  - Zero external dependencies — pure standard library
- Content is organized into **sections**
  - `til/` — short "Today I Learned" notes
  - `articles/` — longer form writing
  - Add more sections just by creating subdirectories in `./content/`
- Frontmatter is optional YAML-like metadata at the top of each file
  ```
  ---
  Title: My Article
  Date: 2026-10-28
  Lang: EN
  Category: Meta
  ---
  ```
  - If absent, the title is derived from the filename and the date from the file modification time
- Writing is done through the built-in admin editor at `/write`
  - Protected by username + password authentication
  - Supports live preview, local draft autosave, and keyboard shortcuts
- TODO Add RSS feed support
- [[GoSeq]] [[Digital Garden]] #goseq #writing
