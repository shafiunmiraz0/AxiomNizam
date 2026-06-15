package apibuilder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ===================================================================
// SQL Assistant — AI-powered SQL suggestions for API Builder
// ===================================================================

func (h *APIBuilderHandler) ChatSQLAssistant(c *gin.Context) {
	var req sqlAssistantChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "details": err.Error()})
		return
	}

	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}
	req.Dialect = normalizeSQLDialect(req.Dialect)
	req.SchemaHint = strings.TrimSpace(req.SchemaHint)
	req.Context = strings.TrimSpace(req.Context)

	assistant := buildFallbackSQLAssistantResponse(req)
	if remoteResp, err := queryOpenClawSQLAssistant(c.Request.Context(), req); err == nil {
		assistant = remoteResp
	} else if strings.TrimSpace(os.Getenv("OPENCLAW_SQL_ASSISTANT_URL")) != "" {
		assistant.Warning = buildSQLAssistantFallbackWarning(err)
		logging.Z().Warn("sql-assistant: OpenClaw request failed, using fallback", zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "assistant": assistant})
}

func buildSQLAssistantFallbackWarning(err error) string {
	if err == nil {
		return "OpenClaw request failed; returned local SQL suggestions."
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "no api key found for provider"),
		strings.Contains(message, "auth store"),
		strings.Contains(message, "provider auth"):
		return "OpenClaw is reachable, but model credentials are not configured; returned local SQL suggestions."
	case strings.Contains(message, "openclaw status=500"),
		strings.Contains(message, "\"type\":\"api_error\""):
		return "OpenClaw is reachable but returned an internal model error (usually missing provider/API-key setup); returned local SQL suggestions."
	case strings.Contains(message, "context deadline exceeded"),
		strings.Contains(message, "context canceled"):
		return "OpenClaw is reachable, but response timed out before completion; returned local SQL suggestions."
	case strings.Contains(message, "connection refused"),
		strings.Contains(message, "dial tcp"),
		strings.Contains(message, "no such host"),
		strings.Contains(message, "i/o timeout"):
		return "OpenClaw endpoint is unreachable; returned local SQL suggestions."
	default:
		return "OpenClaw request failed; returned local SQL suggestions."
	}
}

func queryOpenClawSQLAssistant(ctx context.Context, req sqlAssistantChatRequest) (sqlAssistantChatResponse, error) {
	endpoint := strings.TrimSpace(os.Getenv("OPENCLAW_SQL_ASSISTANT_URL"))
	if endpoint == "" {
		return sqlAssistantChatResponse{}, fmt.Errorf("openclaw endpoint not configured")
	}

	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(resolveSQLAssistantTimeoutSeconds())*time.Second)
	defer cancel()

	model := strings.TrimSpace(os.Getenv("OPENCLAW_SQL_ASSISTANT_MODEL"))
	if model == "" {
		model = "openclaw"
	}

	responseBytes, err := executeOpenClawChatCompletionsRequest(requestCtx, endpoint, strings.TrimSpace(os.Getenv("OPENCLAW_SQL_ASSISTANT_TOKEN")), buildOpenClawSQLAssistantPayload(req, model))
	if err != nil {
		return sqlAssistantChatResponse{}, err
	}

	assistant, err := parseOpenClawSQLAssistantResponse(responseBytes)
	if err != nil {
		return sqlAssistantChatResponse{}, err
	}

	if len(assistant.Suggestions) == 0 {
		fallback := buildFallbackSQLAssistantResponse(req)
		assistant.Suggestions = fallback.Suggestions
		assistant.Warning = "OpenClaw did not return structured suggestions; using local SQL suggestions."
	}

	return assistant, nil
}

func resolveSQLAssistantTimeoutSeconds() int {
	timeoutSeconds := 20
	if raw := strings.TrimSpace(os.Getenv("OPENCLAW_SQL_ASSISTANT_TIMEOUT_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			timeoutSeconds = parsed
		}
	}
	return timeoutSeconds
}

func buildOpenClawSQLAssistantPayload(req sqlAssistantChatRequest, model string) map[string]interface{} {
	return map[string]interface{}{
		"model":       model,
		"temperature": 0.1,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a senior SQL assistant for API builders. Return strict JSON with fields: reply (string), suggestions (array of {title, sql, notes}). Use ? placeholders for parameters.",
			},
			{
				"role":    "user",
				"content": buildSQLAssistantUserPrompt(req),
			},
		},
	}
}

func executeOpenClawChatCompletionsRequest(ctx context.Context, endpoint string, token string, payload map[string]interface{}) ([]byte, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	httpResp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	responseBytes, err := io.ReadAll(io.LimitReader(httpResp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("openclaw status=%d body=%s", httpResp.StatusCode, truncateText(string(responseBytes), 240))
	}

	return responseBytes, nil
}

func parseOpenClawSQLAssistantResponse(responseBytes []byte) (sqlAssistantChatResponse, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBytes, &parsed); err != nil {
		return sqlAssistantChatResponse{}, fmt.Errorf("invalid response JSON: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return sqlAssistantChatResponse{}, fmt.Errorf("no choices returned")
	}

	assistantJSON, err := extractJSONObjectFromText(parsed.Choices[0].Message.Content)
	if err != nil {
		return sqlAssistantChatResponse{}, fmt.Errorf("failed to parse assistant JSON: %w", err)
	}

	var assistant sqlAssistantChatResponse
	if err := json.Unmarshal(assistantJSON, &assistant); err != nil {
		return sqlAssistantChatResponse{}, fmt.Errorf("assistant payload decode failed: %w", err)
	}

	assistant.Provider = "openclaw"
	assistant.Reply = strings.TrimSpace(assistant.Reply)
	assistant.Suggestions = sanitizeSQLAssistantSuggestions(assistant.Suggestions)
	if assistant.Reply == "" {
		assistant.Reply = "Generated SQL suggestions from OpenClaw."
	}

	return assistant, nil
}

func buildSQLAssistantUserPrompt(req sqlAssistantChatRequest) string {
	parts := []string{
		"Generate SQL suggestions for API building.",
		"Dialect: " + normalizeSQLDialect(req.Dialect),
		"Task: " + req.Message,
	}
	if req.SchemaHint != "" {
		parts = append(parts, "Schema hint: "+truncateText(req.SchemaHint, 2400))
	}
	if req.Context != "" {
		parts = append(parts, "Context: "+truncateText(req.Context, 1200))
	}
	parts = append(parts, "Return strict JSON only with keys: reply, suggestions. suggestions items require title and sql.")
	return strings.Join(parts, "\n")
}

func buildFallbackSQLAssistantResponse(req sqlAssistantChatRequest) sqlAssistantChatResponse {
	table := inferSQLTableName(req.Message, req.SchemaHint)
	if table == "" {
		table = "users"
	}
	dialect := normalizeSQLDialect(req.Dialect)
	suggestions := buildFallbackSQLSuggestions(req.Message, dialect, table)

	reply := fmt.Sprintf("Generated %d SQL suggestion(s) for %s. You can apply one directly to the API SQL template.", len(suggestions), dialect)

	return sqlAssistantChatResponse{
		Provider:    "rule-based",
		Reply:       reply,
		Suggestions: suggestions,
	}
}

func buildFallbackSQLSuggestions(message string, dialect string, table string) []sqlAssistantSuggestion {
	lowerMsg := strings.ToLower(message)
	ilikeExpr := "LIKE '%' || ? || '%'"
	if dialect == "mysql" || dialect == "mariadb" || dialect == "percona" {
		ilikeExpr = "LIKE CONCAT('%', ?, '%')"
	}

	base := []sqlAssistantSuggestion{
		{
			Title: "List recent records",
			SQL:   fmt.Sprintf("SELECT * FROM %s ORDER BY created_at DESC LIMIT ? OFFSET ?", table),
			Notes: "Use query params limit:number:false and offset:number:false",
		},
		{
			Title: "Count rows",
			SQL:   fmt.Sprintf("SELECT COUNT(*) AS total FROM %s", table),
			Notes: "Simple aggregate endpoint",
		},
		{
			Title: "Search by name",
			SQL:   fmt.Sprintf("SELECT * FROM %s WHERE name %s ORDER BY created_at DESC LIMIT ?", table, ilikeExpr),
			Notes: "Use query params search:string:false and limit:number:false",
		},
	}

	if containsAny(lowerMsg, []string{"status", "state", "active"}) {
		base = append([]sqlAssistantSuggestion{{
			Title: "Filter by status",
			SQL:   fmt.Sprintf("SELECT * FROM %s WHERE status = ? ORDER BY created_at DESC LIMIT ?", table),
			Notes: "Use query params status:string:true and limit:number:false",
		}}, base...)
	}

	if containsAny(lowerMsg, []string{"id", "single", "detail", "one"}) {
		base = append([]sqlAssistantSuggestion{{
			Title: "Fetch by ID",
			SQL:   fmt.Sprintf("SELECT * FROM %s WHERE id = ? LIMIT 1", table),
			Notes: "Use query param id:number:true",
		}}, base...)
	}

	if containsAny(lowerMsg, []string{"date", "between", "range", "period"}) {
		base = append([]sqlAssistantSuggestion{{
			Title: "Filter by date range",
			SQL:   fmt.Sprintf("SELECT * FROM %s WHERE created_at BETWEEN ? AND ? ORDER BY created_at DESC LIMIT ?", table),
			Notes: "Use query params start:string:true end:string:true limit:number:false",
		}}, base...)
	}

	if len(base) > 5 {
		base = base[:5]
	}
	return base
}

func normalizeSQLDialect(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "mysql", "mariadb", "percona", "postgres", "oracle":
		return s
	default:
		return "postgres"
	}
}

func inferSQLTableName(message string, schemaHint string) string {
	fromPattern := regexp.MustCompile(`(?i)\b(?:from|join|table)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	if matches := fromPattern.FindStringSubmatch(message); len(matches) > 1 {
		return strings.ToLower(strings.TrimSpace(matches[1]))
	}

	if matches := fromPattern.FindStringSubmatch(schemaHint); len(matches) > 1 {
		return strings.ToLower(strings.TrimSpace(matches[1]))
	}

	linePattern := regexp.MustCompile(`(?m)^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*[\(:]`)
	if matches := linePattern.FindStringSubmatch(schemaHint); len(matches) > 1 {
		candidate := strings.ToLower(strings.TrimSpace(matches[1]))
		if candidate != "select" && candidate != "where" {
			return candidate
		}
	}

	return ""
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func sanitizeSQLAssistantSuggestions(values []sqlAssistantSuggestion) []sqlAssistantSuggestion {
	clean := make([]sqlAssistantSuggestion, 0, len(values))
	for _, item := range values {
		title := strings.TrimSpace(item.Title)
		sql := strings.TrimSpace(item.SQL)
		notes := strings.TrimSpace(item.Notes)
		if sql == "" {
			continue
		}
		if title == "" {
			title = "SQL suggestion"
		}
		clean = append(clean, sqlAssistantSuggestion{Title: title, SQL: sql, Notes: notes})
		if len(clean) >= 5 {
			break
		}
	}
	return clean
}

func extractJSONObjectFromText(content string) ([]byte, error) {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return []byte(trimmed), nil
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		return []byte(trimmed[start : end+1]), nil
	}

	return nil, fmt.Errorf("no JSON object found")
}

func truncateText(value string, maxLen int) string {
	v := strings.TrimSpace(value)
	if maxLen <= 0 || len(v) <= maxLen {
		return v
	}
	if maxLen <= 3 {
		return v[:maxLen]
	}
	return v[:maxLen-3] + "..."
}
