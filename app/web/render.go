package web

import (
	"bytes"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/requestctx"
)

type Page struct {
	Title  string
	Nav    string
	Viewer requestctx.Viewer
	Flash  Flash
	Data   any
}

type Renderer struct {
	pages map[string]*template.Template
}

func NewRenderer() (*Renderer, error) {
	base, err := template.New("base").Funcs(funcs()).ParseFS(assets, "templates/*.html", "templates/partials/*.html")
	if err != nil {
		return nil, err
	}
	files, err := fs.Glob(assets, "templates/pages/*.html")
	if err != nil {
		return nil, err
	}
	pages := map[string]*template.Template{}
	for _, file := range files {
		if pages[pageName(file)], err = parsePage(base, file); err != nil {
			return nil, err
		}
	}
	return &Renderer{pages: pages}, nil
}

func parsePage(base *template.Template, file string) (*template.Template, error) {
	clone, err := base.Clone()
	if err != nil {
		return nil, err
	}
	return clone.ParseFS(assets, file)
}

func pageName(file string) string {
	return strings.TrimSuffix(path.Base(file), ".html")
}

func (r *Renderer) Render(c *gin.Context, status int, name string, page Page) {
	page.Viewer = requestctx.ViewerFrom(c)
	page.Flash = TakeFlash(c)
	r.execute(c, status, name, "layout", page)
}

func (r *Renderer) Fragment(c *gin.Context, status int, page, block string, data any) {
	r.execute(c, status, page, block, data)
}

func (r *Renderer) execute(c *gin.Context, status int, name, block string, data any) {
	tmpl, ok := r.pages[name]
	if !ok {
		log.Printf("unknown page template %q", name)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, block, data); err != nil {
		log.Printf("render %s/%s: %v", name, block, err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Data(status, "text/html; charset=utf-8", buf.Bytes())
}
