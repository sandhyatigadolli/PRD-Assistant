// Package api exposes the agent over REST (Gin). The OpenAPI spec lives in
// openapi.yaml and is served with Swagger UI at /swagger.
package api

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"prd-assistant/internal/agent"
	"prd-assistant/internal/config"
	"prd-assistant/internal/domain"
	"prd-assistant/internal/llm"
	"prd-assistant/internal/store"
	"prd-assistant/internal/templates"
	"prd-assistant/internal/tools"
)

//go:embed openapi.yaml
var openapiSpec []byte

type Server struct {
	cfg   config.Config
	agent *agent.Agent
	llm   *llm.Holder
	tpls  *templates.Registry
	tools *tools.Registry
	store store.Store
}

func New(cfg config.Config, st store.Store, tpls *templates.Registry) *gin.Engine {
	h := &llm.Holder{}
	reg := tools.NewRegistry(h)
	s := &Server{cfg: cfg, llm: h, tpls: tpls, tools: reg, store: st,
		agent: &agent.Agent{Store: st, Templates: tpls, Tools: reg, LLM: h}}

	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	r.GET("/openapi.yaml", func(c *gin.Context) { c.Data(200, "application/yaml", openapiSpec) })
	r.GET("/swagger", func(c *gin.Context) { c.Data(200, "text/html; charset=utf-8", []byte(swaggerHTML)) })

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	api.GET("/tags", func(c *gin.Context) { c.JSON(200, domain.Tags) })
	api.GET("/prd-types", s.prdTypes)
	api.GET("/tools", s.listTools)

	api.GET("/llm/status", s.llmStatus)
	api.GET("/llm/local", s.llmLocal)
	api.POST("/llm/configure", s.llmConfigure)
	api.POST("/llm/reset", func(c *gin.Context) { s.llm.Reset(); s.llmStatus(c) })

	api.GET("/sessions", s.listSessions)
	api.POST("/sessions", s.createSession)
	g := api.Group("/sessions/:id")
	g.GET("", s.getSession)
	g.POST("/answer", s.answer)
	g.POST("/skip", s.skip)
	g.POST("/repeat", s.repeat)
	g.POST("/answers/:aid/accept", s.accept)
	g.POST("/answers/:aid/propose", s.propose)
	g.POST("/answers/:aid/apply", s.apply)
	g.GET("/answers/:aid/history", s.history)
	g.GET("/preview", s.preview)
	g.POST("/generate", s.generate)
	g.GET("/export", s.export)
	return r
}

// ---------- helpers ----------

func fail(c *gin.Context, err error) {
	var ue *agent.UserError
	switch {
	case errors.As(err, &ue):
		c.JSON(http.StatusBadRequest, gin.H{"error": ue.Msg})
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func badReq(c *gin.Context, msg string) { c.JSON(http.StatusBadRequest, gin.H{"error": msg}) }

func tagFilter(c *gin.Context) map[string]bool {
	f := map[string]bool{}
	for _, t := range strings.Split(c.Query("tags"), ",") {
		if t = strings.ToUpper(strings.TrimSpace(t)); t != "" {
			f[t] = true
		}
	}
	return f
}

// ---------- meta ----------

func (s *Server) prdTypes(c *gin.Context) {
	out := []gin.H{}
	for _, t := range s.tpls.List() {
		secs := []string{}
		for _, sec := range t.Sections {
			secs = append(secs, sec.Title)
		}
		out = append(out, gin.H{"type": t.Type, "title": t.Title, "description": t.Description, "sections": secs})
	}
	c.JSON(200, out)
}

func (s *Server) listTools(c *gin.Context) {
	out := []gin.H{}
	for _, t := range s.tools.List() {
		out = append(out, gin.H{"name": t.Name(), "description": t.Description()})
	}
	c.JSON(200, out)
}

// ---------- LLM gate ----------

func (s *Server) llmStatus(c *gin.Context) {
	cl := s.llm.Get()
	resp := gin.H{"ready": cl != nil, "default_local_url": s.cfg.OllamaURL}
	if cl != nil {
		resp["info"] = cl.Describe() // never includes the API key
	}
	c.JSON(200, resp)
}

func (s *Server) llmLocal(c *gin.Context) {
	base := strings.TrimRight(c.DefaultQuery("base_url", s.cfg.OllamaURL), "/")
	models, err := llm.LocalModels(c.Request.Context(), base)
	if err != nil {
		c.JSON(200, gin.H{"reachable": false, "error": err.Error(), "models": []string{}})
		return
	}
	c.JSON(200, gin.H{"reachable": true, "models": models})
}

func (s *Server) llmConfigure(c *gin.Context) {
	var cfg llm.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		badReq(c, "invalid JSON body")
		return
	}
	if cfg.Mode == "local" && cfg.BaseURL == "" {
		cfg.BaseURL = s.cfg.OllamaURL
	}
	cl, err := s.connect(c.Request.Context(), cfg)
	if err != nil {
		s.llm.Reset()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "hint": failureHint(cfg, err)})
		return
	}
	s.llm.Set(cl)
	s.llmStatus(c)
}

// ---------- sessions ----------

func (s *Server) listSessions(c *gin.Context) {
	list, err := s.store.ListSessions(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, list)
}

func (s *Server) createSession(c *gin.Context) {
	var in struct {
		PRDType string `json:"prd_type"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badReq(c, "invalid JSON body")
		return
	}
	st, err := s.agent.StartSession(c.Request.Context(), in.PRDType)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(201, st)
}

func (s *Server) getSession(c *gin.Context) {
	st, err := s.agent.GetState(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, st)
}

func (s *Server) answer(c *gin.Context) {
	var in struct {
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badReq(c, "invalid JSON body")
		return
	}
	st, err := s.agent.Submit(c.Request.Context(), c.Param("id"), in.Content, in.Tags)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, st)
}

func (s *Server) skip(c *gin.Context) {
	var in struct {
		Scope string `json:"scope"` // question (default) | section
	}
	_ = c.ShouldBindJSON(&in)
	st, err := s.agent.Skip(c.Request.Context(), c.Param("id"), in.Scope == "section")
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, st)
}

func (s *Server) repeat(c *gin.Context) {
	var in struct {
		Add bool `json:"add"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badReq(c, "invalid JSON body")
		return
	}
	st, err := s.agent.Repeat(c.Request.Context(), c.Param("id"), in.Add)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, st)
}

func (s *Server) accept(c *gin.Context) {
	st, err := s.agent.Accept(c.Request.Context(), c.Param("id"), c.Param("aid"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, st)
}

func (s *Server) propose(c *gin.Context) {
	var in agent.ProposeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badReq(c, "invalid JSON body")
		return
	}
	p, err := s.agent.Propose(c.Request.Context(), c.Param("id"), c.Param("aid"), in)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, p)
}

func (s *Server) apply(c *gin.Context) {
	var in struct {
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badReq(c, "invalid JSON body")
		return
	}
	a, err := s.agent.Apply(c.Request.Context(), c.Param("id"), c.Param("aid"), in.Content, in.Tags)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, a)
}

func (s *Server) history(c *gin.Context) {
	v, err := s.agent.History(c.Request.Context(), c.Param("id"), c.Param("aid"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, v)
}

func (s *Server) preview(c *gin.Context) {
	p, err := s.agent.Preview(c.Request.Context(), c.Param("id"), tagFilter(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, p)
}

func (s *Server) generate(c *gin.Context) {
	md, err := s.agent.Generate(c.Request.Context(), c.Param("id"), tagFilter(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"markdown": md})
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (s *Server) export(c *gin.Context) {
	ctx := c.Request.Context()
	sid := c.Param("id")
	format := c.DefaultQuery("format", "docx")
	sess, err := s.store.GetSession(ctx, sid)
	if err != nil {
		fail(c, err)
		return
	}
	md, err := s.exportSource(ctx, sid, c.DefaultQuery("source", "answers"), sess, tagFilter(c))
	if err != nil {
		fail(c, err)
		return
	}
	data, ctype, ext, err := s.tools.Exporter.Export(format, md)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	name := strings.Trim(unsafeName.ReplaceAllString(sess.Title, "-"), "-")
	if name == "" {
		name = "PRD"
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-PRD.%s"`, name, ext))
	c.Data(200, ctype, data)
}

func (s *Server) exportSource(ctx context.Context, sid, source string, sess *domain.Session, f map[string]bool) (string, error) {
	if source == "polished" {
		if sess.Generated == "" {
			return "", &agent.UserError{Msg: "no polished PRD yet. Click 'Generate polished PRD' first"}
		}
		return sess.Generated, nil
	}
	p, err := s.agent.Preview(ctx, sid, f)
	if err != nil {
		return "", err
	}
	return p.Markdown, nil
}

// connect builds and verifies a client. With no model chosen on a cloud
// provider, it discovers the models the key can use and tries the best few.
func (s *Server) connect(ctx context.Context, cfg llm.Config) (llm.Client, error) {
	models := []string{cfg.Model}
	if cfg.Mode == "api" && strings.TrimSpace(cfg.Model) == "" && cfg.Provider != "anthropic" {
		if c := llm.Candidates(ctx, cfg); len(c) > 0 {
			models = c
		}
	}
	var lastErr error
	for _, m := range models {
		cfg.Model = m
		cl, err := llm.New(cfg)
		if err == nil {
			if err = llm.Verify(ctx, cl); err == nil {
				return cl, nil
			}
		}
		lastErr = err
		if strings.Contains(err.Error(), "API key was rejected") {
			break // a different model will not help
		}
	}
	return nil, lastErr
}

func failureHint(cfg llm.Config, err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "429") && (cfg.Provider == "openai" || cfg.Provider == "anthropic"):
		return "OpenAI and Anthropic have no free API tier: the account needs prepaid credit. For a free option choose Google Gemini or Groq."
	case strings.Contains(msg, "429"):
		return "The free quota for this key is used up for now. Wait a minute and try again, or try another free provider (Groq, OpenRouter)."
	case strings.Contains(msg, "API key was rejected"):
		return "Check that you copied the whole key and selected the matching provider."
	}
	return "Either a working API key or a running local LLM is required to build a PRD."
}
