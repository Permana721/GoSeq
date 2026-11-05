package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"strings"
)

//go:embed static/* templates/*
var FS embed.FS

var funcMap = template.FuncMap{
	"lower": strings.ToLower,
}

type Templates struct {
	indexTmpl    *template.Template
	categoryTmpl *template.Template
	articleTmpl  *template.Template
	mainSiteTmpl *template.Template
	loginTmpl    *template.Template
	writeTmpl    *template.Template
}

func NewTemplates() (*Templates, error) {
	indexTmpl, err := template.New("").Funcs(funcMap).ParseFS(FS, "templates/layout.html", "templates/home.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse index template: %w", err)
	}

	categoryTmpl, err := template.New("").Funcs(funcMap).ParseFS(FS, "templates/layout.html", "templates/category.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse category template: %w", err)
	}

	articleTmpl, err := template.New("").Funcs(funcMap).ParseFS(FS, "templates/layout.html", "templates/article.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse article template: %w", err)
	}

	mainSiteTmpl, err := template.New("main-site.html").Funcs(funcMap).ParseFS(FS, "templates/main-site.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse main-site template: %w", err)
	}

	loginTmpl, err := template.New("").Funcs(funcMap).ParseFS(FS, "templates/layout.html", "templates/login.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse login template: %w", err)
	}

	writeTmpl, err := template.New("").Funcs(funcMap).ParseFS(FS, "templates/layout.html", "templates/write.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse write template: %w", err)
	}

	return &Templates{
		indexTmpl:    indexTmpl,
		categoryTmpl: categoryTmpl,
		articleTmpl:  articleTmpl,
		mainSiteTmpl: mainSiteTmpl,
		loginTmpl:    loginTmpl,
		writeTmpl:    writeTmpl,
	}, nil
}

func (t *Templates) RenderIndex(w io.Writer, data any) error {
	return t.indexTmpl.ExecuteTemplate(w, "layout", data)
}

func (t *Templates) RenderCategory(w io.Writer, data any) error {
	return t.categoryTmpl.ExecuteTemplate(w, "layout", data)
}

func (t *Templates) RenderArticle(w io.Writer, data any) error {
	return t.articleTmpl.ExecuteTemplate(w, "layout", data)
}

func (t *Templates) RenderMainSite(w io.Writer, data any) error {
	return t.mainSiteTmpl.Execute(w, data)
}

func (t *Templates) RenderLogin(w io.Writer, data any) error {
	return t.loginTmpl.ExecuteTemplate(w, "layout", data)
}

func (t *Templates) RenderWrite(w io.Writer, data any) error {
	return t.writeTmpl.ExecuteTemplate(w, "layout", data)
}
