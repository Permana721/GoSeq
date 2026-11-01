---
Title: Arsitektur GoSeq: Zero-Dependency di Golang
Date: 2026-10-30
Lang: ID
Category: Rekayasa Perangkat Lunak
---
- GoSeq dibangun menggunakan 100% Go Standard Library tanpa pustaka eksternal pihak ketiga
  - Menggunakan paket `net/http` untuk perutean HTTP standar Go 1.22+
  - Menggunakan `html/template` dan `embed` untuk menyatukan HTML dan aset statis ke dalam satu berkas biner
- Model Hierarki 3-Tingkat
  - Tingkat 1: Halaman beranda dengan 2 kartu utama (Articles dan Today I Learned)
  - Tingkat 2: Tampilan daftar kategori (`/category/{slug}`) yang minimalis
  - Tingkat 3: Tampilan detail artikel (`/read/{category}/{filename}`) dengan gaya Logseq outliner
- TODO Implementasi pencarian berbasis teks penuh di tingkat lokal
- DONE Desain sistem tema adaptif otomatis (Light & Dark Mode)
- [[GoSeq]] [[Golang]] #arsitektur #golang
