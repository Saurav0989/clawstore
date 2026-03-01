package mcpserver

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/server"
)

type Server struct {
	MCP        *server.MCPServer
	Streamable *server.StreamableHTTPServer
	token      string
	noAuth     bool
}

func New(deps ToolDeps, token string, noAuth bool) *Server {
	m := server.NewMCPServer(
		"clawstore",
		"0.1.0",
		server.WithToolCapabilities(true),
	)
	RegisterTools(m, deps)
	h := server.NewStreamableHTTPServer(
		m,
		server.WithEndpointPath("/mcp"),
		server.WithStateLess(true),
	)
	return &Server{MCP: m, Streamable: h, token: token, noAuth: noAuth}
}

func (s *Server) Handler() http.Handler {
	if s.noAuth {
		return s.Streamable
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r.Header.Get("Authorization")) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":401,"error":"unauthorized"}`))
			return
		}
		s.Streamable.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(header string) bool {
	if s.noAuth {
		return true
	}
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if provided == "" || strings.TrimSpace(s.token) == "" {
		return false
	}
	if len(provided) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) == 1
}
