package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/saurav0989/clawstore/embed"
	"github.com/saurav0989/clawstore/store"
	"go.uber.org/zap"
)

type ToolDeps struct {
	DB       *store.DB
	Embedder embed.Embedder
	Logger   *zap.Logger
}

func RegisterTools(s *server.MCPServer, deps ToolDeps) {
	registerMemoryWrite(s, deps)
	registerMemoryRead(s, deps)
	registerMemorySearch(s, deps)
	registerMemoryLogAction(s, deps)
	registerEntityList(s, deps)
	registerMemoryRecent(s, deps)
}

func registerMemoryWrite(s *server.MCPServer, deps ToolDeps) {
	tool := mcp.NewTool("memory_write",
		mcp.WithDescription("Store an observation or fact in long-term memory"),
		mcp.WithString("content", mcp.Required(), mcp.Description("the fact to remember")),
		mcp.WithString("entity_id", mcp.Description("slug of the entity this is about")),
		mcp.WithString("source", mcp.Description("source label"), mcp.DefaultString("agent")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		entityID := req.GetString("entity_id", "")
		source := req.GetString("source", "agent")
		obs, err := deps.DB.AddObservationWithEmbedding(ctx, deps.Embedder, entityID, content, source, 1.0)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		entityLabel := obs.EntityID
		if entityLabel == "" {
			entityLabel = "general"
		}
		return mcp.NewToolResultStructuredOnly(map[string]any{
			"id":         obs.ID,
			"entity_id":  entityLabel,
			"created_at": obs.CreatedAt,
		}), nil
	})
}

func registerMemoryRead(s *server.MCPServer, deps ToolDeps) {
	tool := mcp.NewTool("memory_read",
		mcp.WithDescription("Get all observations about a specific entity"),
		mcp.WithString("entity_id", mcp.Required()),
		mcp.WithNumber("limit", mcp.DefaultNumber(20)),
		mcp.WithNumber("since_days", mcp.Description("optional lookback window in days")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		entityID, err := req.RequireString("entity_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := req.GetInt("limit", 20)
		if limit <= 0 || limit > 1000 {
			return mcp.NewToolResultError("limit must be between 1 and 1000"), nil
		}
		sinceDays := req.GetInt("since_days", 0)
		if sinceDays < 0 {
			return mcp.NewToolResultError("since_days must be >= 0"), nil
		}
		var since *time.Time
		if sinceDays > 0 {
			t := time.Now().Add(-time.Duration(sinceDays) * 24 * time.Hour)
			since = &t
		}

		entity, err := deps.DB.GetEntity(ctx, store.Slugify(entityID))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		observations, err := deps.DB.ReadObservationsByEntity(ctx, entity.ID, limit, since)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		items := make([]map[string]any, 0, len(observations))
		for _, o := range observations {
			items = append(items, map[string]any{
				"id":         o.ID,
				"content":    o.Content,
				"source":     o.Source,
				"created_at": o.CreatedAt,
			})
		}
		return mcp.NewToolResultStructuredOnly(map[string]any{
			"observations": items,
			"entity": map[string]any{
				"id":   entity.ID,
				"name": entity.Name,
				"type": entity.Type,
			},
		}), nil
	})
}

func registerMemorySearch(s *server.MCPServer, deps ToolDeps) {
	tool := mcp.NewTool("memory_search",
		mcp.WithDescription("Search long-term memory by meaning or keywords"),
		mcp.WithString("query", mcp.Required()),
		mcp.WithString("mode", mcp.DefaultString("hybrid"), mcp.Enum("hybrid", "semantic", "fts")),
		mcp.WithNumber("limit", mcp.DefaultNumber(10)),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		mode := req.GetString("mode", "hybrid")
		if _, ok := validSearchModes[store.ParseSearchMode(mode)]; !ok {
			return mcp.NewToolResultError("mode must be one of: hybrid, semantic, fts"), nil
		}
		limit := req.GetInt("limit", 10)
		if limit <= 0 || limit > 1000 {
			return mcp.NewToolResultError("limit must be between 1 and 1000"), nil
		}
		results, err := deps.DB.Search(ctx, deps.Embedder, query, store.ParseSearchMode(mode), limit)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		_, _ = deps.DB.AppendActionLog(ctx, "openclaw", "tool_call", "search: "+query, map[string]any{
			"mode":  mode,
			"limit": limit,
		}, nil)
		items := make([]map[string]any, 0, len(results))
		for _, r := range results {
			entityID := r.EntityID
			if strings.TrimSpace(entityID) == "" {
				entityID = "general"
			}
			items = append(items, map[string]any{
				"content":     r.Content,
				"entity_id":   entityID,
				"entity_name": r.EntityName,
				"score":       r.Score,
				"created_at":  r.CreatedAt,
			})
		}
		return mcp.NewToolResultStructuredOnly(map[string]any{"results": items}), nil
	})
}

func registerMemoryLogAction(s *server.MCPServer, deps ToolDeps) {
	tool := mcp.NewTool("memory_log_action",
		mcp.WithDescription("Log an agent action, decision, or tool call for auditing and context"),
		mcp.WithString("action_type", mcp.Required(), mcp.Enum("tool_call", "decision", "error", "cron_run")),
		mcp.WithString("summary", mcp.Required()),
		mcp.WithObject("detail", mcp.Description("optional JSON object detail")),
		mcp.WithArray("entity_ids", mcp.WithStringItems()),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		actionType, err := req.RequireString("action_type")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if _, ok := mcpActionTypes[actionType]; !ok {
			return mcp.NewToolResultError("action_type must be one of: tool_call, decision, error, cron_run"), nil
		}
		summary, err := req.RequireString("summary")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		entityIDs := req.GetStringSlice("entity_ids", nil)
		for _, entityID := range entityIDs {
			if strings.TrimSpace(entityID) == "" {
				return mcp.NewToolResultError("entity_ids must not contain empty values"), nil
			}
		}
		var detail any
		if raw := req.GetArguments(); raw != nil {
			detail = raw["detail"]
		}
		id, err := deps.DB.AppendActionLog(ctx, "openclaw", actionType, summary, detail, entityIDs)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultStructuredOnly(map[string]any{"id": id}), nil
	})
}

func registerEntityList(s *server.MCPServer, deps ToolDeps) {
	tool := mcp.NewTool("entity_list",
		mcp.WithDescription("List all known entities in memory"),
		mcp.WithString("type", mcp.Enum("person", "project", "place", "preference", "concept")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		typeFilter := req.GetString("type", "")
		entities, err := deps.DB.ListEntities(ctx, typeFilter)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		items := make([]map[string]any, 0, len(entities))
		for _, e := range entities {
			items = append(items, map[string]any{
				"id":                e.ID,
				"name":              e.Name,
				"type":              e.Type,
				"observation_count": e.ObservationCount,
				"updated_at":        e.UpdatedAt,
			})
		}
		return mcp.NewToolResultStructuredOnly(map[string]any{"entities": items}), nil
	})
}

func registerMemoryRecent(s *server.MCPServer, deps ToolDeps) {
	tool := mcp.NewTool("memory_recent",
		mcp.WithDescription("Get the most recent observations across all entities"),
		mcp.WithNumber("limit", mcp.DefaultNumber(20)),
		mcp.WithNumber("since_hours", mcp.Description("optional lookback in hours")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		limit := req.GetInt("limit", 20)
		if limit <= 0 || limit > 1000 {
			return mcp.NewToolResultError("limit must be between 1 and 1000"), nil
		}
		sinceHours := req.GetInt("since_hours", 0)
		if sinceHours < 0 {
			return mcp.NewToolResultError("since_hours must be >= 0"), nil
		}
		var since *time.Time
		if sinceHours == 0 {
			sinceHours = 24
		}
		if sinceHours > 0 {
			t := time.Now().Add(-time.Duration(sinceHours) * time.Hour)
			since = &t
		}
		observations, err := deps.DB.RecentObservations(ctx, limit, since)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		items := make([]map[string]any, 0, len(observations))
		for _, o := range observations {
			entityID := o.EntityID
			if strings.TrimSpace(entityID) == "" {
				entityID = "general"
			}
			items = append(items, map[string]any{
				"content":     o.Content,
				"entity_id":   entityID,
				"entity_name": o.EntityName,
				"source":      o.Source,
				"created_at":  o.CreatedAt,
			})
		}
		return mcp.NewToolResultStructuredOnly(map[string]any{"observations": items}), nil
	})
}

var validSearchModes = map[store.SearchMode]struct{}{
	store.SearchModeHybrid:   {},
	store.SearchModeSemantic: {},
	store.SearchModeFTS:      {},
}

var mcpActionTypes = map[string]struct{}{
	"tool_call": {},
	"decision":  {},
	"error":     {},
	"cron_run":  {},
}

func badRequest(msg string, args ...any) *mcp.CallToolResult {
	return mcp.NewToolResultError(fmt.Sprintf(msg, args...))
}
