package codexhistory

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

func GetThreadDetail(indexDB string, id string, maxItemBytes int) (ThreadDetail, error) {
	indexDB, err := normalizeIndexDB(indexDB)
	if err != nil {
		return ThreadDetail{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ThreadDetail{}, fmt.Errorf("thread id is required")
	}
	if maxItemBytes <= 0 {
		maxItemBytes = 120_000
	}

	thread, err := loadIndexedThread(indexDB, id)
	if err != nil {
		return ThreadDetail{}, err
	}
	detail := ThreadDetail{Thread: thread}
	if thread.RolloutPath == "" {
		return detail, nil
	}
	items, err := readThreadItems(thread.RolloutPath, maxItemBytes)
	if err != nil {
		return ThreadDetail{}, err
	}
	detail.Items = items
	return detail, nil
}

func loadIndexedThread(indexDB string, id string) (Thread, error) {
	db, err := sql.Open("sqlite", indexDB)
	if err != nil {
		return Thread{}, err
	}
	defer db.Close()

	var thread Thread
	var createdAt, updatedAt int64
	var archived int
	var createdAtText, updatedAtText string
	err = db.QueryRow(`
		select
			id, title, cwd, rollout_path, source,
			created_at, updated_at, created_at_ms, updated_at_ms,
			archived, first_user_message, preview, codex_url
		from threads
		where id = ?`, id).Scan(
		&thread.ID,
		&thread.Title,
		&thread.CWD,
		&thread.RolloutPath,
		&thread.Source,
		&createdAtText,
		&updatedAtText,
		&createdAt,
		&updatedAt,
		&archived,
		&thread.FirstUserMessage,
		&thread.Preview,
		&thread.CodexURL,
	)
	if err == sql.ErrNoRows {
		return Thread{}, fmt.Errorf("thread not found: %s", id)
	}
	if err != nil {
		return Thread{}, err
	}
	thread.CreatedAt = TimeFromUnixish(createdAt)
	thread.UpdatedAt = TimeFromUnixish(updatedAt)
	thread.Archived = archived != 0
	if thread.CodexURL == "" {
		thread.CodexURL = "codex://threads/" + thread.ID
	}
	_ = createdAtText
	_ = updatedAtText
	return thread, nil
}

func readThreadItems(path string, maxItemBytes int) ([]ThreadItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open rollout: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	var items []ThreadItem
	for scanner.Scan() {
		item, ok := parseThreadLine(scanner.Bytes(), maxItemBytes)
		if ok {
			items = append(items, item)
		}
	}
	if err := scanner.Err(); err != nil {
		return items, err
	}
	return items, nil
}

func parseThreadLine(line []byte, maxItemBytes int) (ThreadItem, bool) {
	var record struct {
		Timestamp string         `json:"timestamp"`
		Type      string         `json:"type"`
		Payload   map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(line, &record); err != nil {
		text, truncated := truncateTextBytes(string(line), maxItemBytes)
		return ThreadItem{Kind: "raw", Title: "unparsed line", Text: text, Truncated: truncated}, true
	}
	if record.Payload == nil {
		return ThreadItem{}, false
	}
	switch record.Type {
	case "response_item":
		return parseResponseItem(record.Timestamp, record.Payload, maxItemBytes)
	case "event_msg":
		return parseEventMessage(record.Timestamp, record.Payload, maxItemBytes)
	default:
		return ThreadItem{}, false
	}
}

func parseResponseItem(timestamp string, payload map[string]any, maxItemBytes int) (ThreadItem, bool) {
	switch stringField(payload, "type") {
	case "message":
		text := contentText(payload["content"])
		if strings.TrimSpace(text) == "" {
			return ThreadItem{}, false
		}
		text, truncated := truncateTextBytes(text, maxItemBytes)
		role := stringField(payload, "role")
		title := role
		if phase := stringField(payload, "phase"); phase != "" {
			title += " / " + phase
		}
		return ThreadItem{
			Timestamp: timestamp,
			Kind:      "message",
			Role:      role,
			Title:     title,
			Text:      text,
			Truncated: truncated,
		}, true
	case "function_call":
		name := stringField(payload, "name")
		args := stringField(payload, "arguments")
		if strings.TrimSpace(args) == "" {
			return ThreadItem{}, false
		}
		text, truncated := truncateTextBytes(args, maxItemBytes)
		return ThreadItem{
			Timestamp: timestamp,
			Kind:      "tool",
			Title:     "tool call: " + name,
			Text:      text,
			Truncated: truncated,
		}, true
	case "function_call_output":
		output := stringField(payload, "output")
		if strings.TrimSpace(output) == "" {
			return ThreadItem{}, false
		}
		text, truncated := truncateTextBytes(output, maxItemBytes)
		return ThreadItem{
			Timestamp: timestamp,
			Kind:      "tool-output",
			Title:     "tool output",
			Text:      text,
			Truncated: truncated,
		}, true
	case "reasoning":
		text := reasoningSummary(payload["summary"])
		if strings.TrimSpace(text) == "" {
			return ThreadItem{}, false
		}
		text, truncated := truncateTextBytes(text, maxItemBytes)
		return ThreadItem{
			Timestamp: timestamp,
			Kind:      "reasoning",
			Title:     "reasoning summary",
			Text:      text,
			Truncated: truncated,
		}, true
	default:
		return ThreadItem{}, false
	}
}

func parseEventMessage(timestamp string, payload map[string]any, maxItemBytes int) (ThreadItem, bool) {
	eventType := stringField(payload, "type")
	switch eventType {
	case "task_started", "turn_aborted", "token_count":
		return ThreadItem{}, false
	case "user_message", "agent_message":
		return ThreadItem{}, false
	}
	text := stringField(payload, "message")
	if text == "" {
		text = stringField(payload, "text")
	}
	if strings.TrimSpace(text) == "" {
		return ThreadItem{}, false
	}
	text, truncated := truncateTextBytes(text, maxItemBytes)
	return ThreadItem{
		Timestamp: timestamp,
		Kind:      "event",
		Title:     eventType,
		Text:      text,
		Truncated: truncated,
	}, true
}

func contentText(value any) string {
	parts, ok := value.([]any)
	if !ok {
		return ""
	}
	var out []string
	for _, part := range parts {
		m, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if text := stringField(m, "text"); text != "" {
			out = append(out, text)
			continue
		}
		switch stringField(m, "type") {
		case "input_image", "image_url":
			out = append(out, "[image omitted]")
		}
	}
	return strings.Join(out, "\n\n")
}

func reasoningSummary(value any) string {
	parts, ok := value.([]any)
	if !ok {
		return ""
	}
	var out []string
	for _, part := range parts {
		if m, ok := part.(map[string]any); ok {
			if text := stringField(m, "text"); text != "" {
				out = append(out, text)
			}
		}
	}
	return strings.Join(out, "\n\n")
}

func stringField(m map[string]any, key string) string {
	value, ok := m[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func truncateTextBytes(value string, maxBytes int) (string, bool) {
	if len(value) <= maxBytes {
		return value, false
	}
	if maxBytes <= 1 {
		return "", true
	}
	var builder strings.Builder
	for _, r := range value {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = len(string(r))
		}
		if builder.Len()+size > maxBytes-1 {
			break
		}
		builder.WriteRune(r)
	}
	builder.WriteString("…")
	return builder.String(), true
}
