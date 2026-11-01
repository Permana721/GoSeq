package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
)

//go:embed static/* templates/*
var FS embed.FS

type Templates struct {
	indexTmpl   *template.Template
	journalTmpl *template.Template
	loginTmpl   *template.Template
	writeTmpl   *template.Template
}

func NewTemplates() (*Templates, error) {
	indexTmpl, err := template.ParseFS(FS, "templates/layout.html", "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse index template: %w", err)
	}

	journalTmpl, err := template.ParseFS(FS, "templates/layout.html", "templates/journal.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse journal template: %w", err)
	}

	loginTmpl, err := template.ParseFS(FS, "templates/layout.html", "templates/login.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse login template: %w", err)
	}

	writeTmpl, err := template.ParseFS(FS, "templates/layout.html", "templates/write.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse write template: %w", err)
	}

	return &Templates{
		indexTmpl:   indexTmpl,
		journalTmpl: journalTmpl,
		loginTmpl:   loginTmpl,
		writeTmpl:   writeTmpl,
	}, nil
}

func (t *Templates) RenderIndex(w io.Writer, data any) error {
	return t.indexTmpl.ExecuteTemplate(w, "layout", data)
}

func (t *Templates) RenderJournal(w io.Writer, data any) error {
	return t.journalTmpl.ExecuteTemplate(w, "layout", data)
}

func (t *Templates) RenderLogin(w io.Writer, data any) error {
	return t.loginTmpl.ExecuteTemplate(w, "layout", data)
}

func (t *Templates) RenderWrite(w io.Writer, data any) error {
	return t.writeTmpl.ExecuteTemplate(w, "layout", data)
}
