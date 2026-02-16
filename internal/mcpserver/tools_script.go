package mcpserver

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerScriptTools adds script management tools to the MCP server.
func (s *Server) registerScriptTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_scripts",
		Description: "List all scripts (active, disabled, and errored).",
	}, s.handleListScripts)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_script",
		Description: "Create a new JS script with YAML header, match patterns, and source code.",
	}, s.handleCreateScript)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_script",
		Description: "Get the source code and metadata of a script by file path.",
	}, s.handleGetScript)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "toggle_script",
		Description: "Toggle a script's enabled/disabled state.",
	}, s.handleToggleScript)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_script",
		Description: "Delete a script file.",
	}, s.handleDeleteScript)
}

// --- list_scripts ---

type listScriptsInput struct{}

type scriptSummary struct {
	Name       string   `json:"name"`
	FilePath   string   `json:"file_path"`
	Matches    []string `json:"match_patterns"`
	Enabled    bool     `json:"enabled"`
	Category   string   `json:"category"`
	Categories []string `json:"categories,omitempty"`
}

func (s *Server) handleListScripts(
	_ context.Context, _ *mcp.CallToolRequest, _ listScriptsInput,
) (*mcp.CallToolResult, any, error) {
	if s.cfg.Scripts == nil {
		return errorResult("scripts not available"), nil, nil
	}

	infos := s.cfg.Scripts.Scripts()
	items := make([]scriptSummary, len(infos))
	for i, info := range infos {
		items[i] = infoToSummary(info)
	}

	return jsonResult(items), nil, nil
}

// --- create_script ---

type createScriptInput struct {
	Name          string   `json:"name" jsonschema:"script name (required)"`
	MatchPatterns []string `json:"match_patterns" jsonschema:"URL patterns to match (required)"`
	Code          string   `json:"code" jsonschema:"JS source code (required)"`
	Enabled       bool     `json:"enabled,omitempty" jsonschema:"enabled state (default true)"`
}

func (s *Server) handleCreateScript(
	_ context.Context, _ *mcp.CallToolRequest, in createScriptInput,
) (*mcp.CallToolResult, any, error) {
	if s.cfg.Scripts == nil {
		return errorResult("scripts not available"), nil, nil
	}
	if in.Name == "" {
		return errorResult("name is required"), nil, nil
	}
	if len(in.MatchPatterns) == 0 {
		return errorResult("match_patterns is required"), nil, nil
	}
	if in.Code == "" {
		return errorResult("code is required"), nil, nil
	}

	// Build match lines.
	var matchLines []string
	for _, p := range in.MatchPatterns {
		matchLines = append(matchLines, fmt.Sprintf(`//   - "%s"`, p))
	}

	content := fmt.Sprintf(`// ---
// name: %s
// match:
%s
// enabled: %v
// ---

%s
`, in.Name, strings.Join(matchLines, "\n"), in.Enabled, in.Code)

	path, err := writeScriptFile(s.cfg.Scripts.ScriptDir(), "script-*.js", content)
	if err != nil {
		return errorResult(fmt.Sprintf("create script: %v", err)), nil, nil
	}
	s.cfg.Scripts.Reload()

	return jsonResult(map[string]any{
		"file_path": path,
	}), nil, nil
}

// --- get_script ---

type getScriptInput struct {
	FilePath string `json:"file_path" jsonschema:"path to script file (required)"`
}

func (s *Server) handleGetScript(
	_ context.Context, _ *mcp.CallToolRequest, in getScriptInput,
) (*mcp.CallToolResult, any, error) {
	if in.FilePath == "" {
		return errorResult("file_path is required"), nil, nil
	}

	data, err := os.ReadFile(in.FilePath) // #nosec G304 -- user-provided script path
	if err != nil {
		return errorResult(fmt.Sprintf("read: %v", err)), nil, nil
	}

	source := string(data)
	meta, body, parseErr := scripting.ParseHeader(source)
	if parseErr != nil {
		return errorResult(fmt.Sprintf("parse: %v", parseErr)), nil, nil
	}

	cats := scripting.DetectCategories(body)
	catStrs := make([]string, len(cats))
	for i, c := range cats {
		catStrs[i] = string(c)
	}

	return jsonResult(map[string]any{
		"name":           meta.Name,
		"match_patterns": meta.Match,
		"enabled":        meta.IsEnabled(),
		"categories":     catStrs,
		"source":         body,
	}), nil, nil
}

// --- toggle_script ---

type toggleScriptInput struct {
	FilePath string `json:"file_path" jsonschema:"path to script file (required)"`
}

func (s *Server) handleToggleScript(
	_ context.Context, _ *mcp.CallToolRequest, in toggleScriptInput,
) (*mcp.CallToolResult, any, error) {
	if s.cfg.Scripts == nil {
		return errorResult("scripts not available"), nil, nil
	}
	if in.FilePath == "" {
		return errorResult("file_path is required"), nil, nil
	}

	if err := s.cfg.Scripts.Toggle(in.FilePath); err != nil {
		return errorResult(fmt.Sprintf("toggle: %v", err)), nil, nil
	}

	// Read back current state.
	data, err := os.ReadFile(in.FilePath) // #nosec G304
	if err != nil {
		return jsonResult(map[string]any{"toggled": true}), nil, nil
	}
	meta, _, _ := scripting.ParseHeader(string(data))
	enabled := true
	if meta != nil {
		enabled = meta.IsEnabled()
	}

	return jsonResult(map[string]any{"enabled": enabled}), nil, nil
}

// --- delete_script ---

type deleteScriptInput struct {
	FilePath string `json:"file_path" jsonschema:"path to script file (required)"`
}

func (s *Server) handleDeleteScript(
	_ context.Context, _ *mcp.CallToolRequest, in deleteScriptInput,
) (*mcp.CallToolResult, any, error) {
	if s.cfg.Scripts == nil {
		return errorResult("scripts not available"), nil, nil
	}
	if in.FilePath == "" {
		return errorResult("file_path is required"), nil, nil
	}

	if err := s.cfg.Scripts.Delete(in.FilePath); err != nil {
		return errorResult(fmt.Sprintf("delete: %v", err)), nil, nil
	}

	return jsonResult(map[string]any{"deleted": true}), nil, nil
}

// --- helpers ---

func infoToSummary(info scripting.ScriptInfo) scriptSummary {
	cats := make([]string, len(info.Categories))
	primary := "script"
	for i, c := range info.Categories {
		cats[i] = string(c)
		if i == 0 {
			primary = strings.ToLower(string(c))
		}
	}
	return scriptSummary{
		Name:       info.Name,
		FilePath:   info.FilePath,
		Matches:    info.Matches,
		Enabled:    info.Enabled,
		Category:   primary,
		Categories: cats,
	}
}

// writeScriptFile creates a temp file in dir with the given prefix and content.
func writeScriptFile(dir, prefix, content string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, prefix)
	if err != nil {
		return "", err
	}
	path := f.Name()
	_, writeErr := f.WriteString(content)
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return path, nil
}
