package codexhistory

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type IndexOptions struct {
	CodexHome        string
	IndexDB          string
	SessionTextBytes int
}

type IndexStats struct {
	Threads       int
	SessionFiles  int
	IndexedAt     time.Time
	IndexDB       string
	StateSnapshot string
}

func BuildIndex(opts IndexOptions) (IndexStats, error) {
	codexHome, err := normalizeCodexHome(opts.CodexHome)
	if err != nil {
		return IndexStats{}, err
	}
	indexDB, err := normalizeIndexDB(opts.IndexDB)
	if err != nil {
		return IndexStats{}, err
	}
	if opts.SessionTextBytes <= 0 {
		opts.SessionTextBytes = 300_000
	}
	if err := os.MkdirAll(filepath.Dir(indexDB), 0o755); err != nil {
		return IndexStats{}, err
	}

	snapshot, err := backupStateDB(codexHome)
	if err != nil {
		return IndexStats{}, err
	}
	defer os.Remove(snapshot)

	threads, err := loadThreads(snapshot)
	if err != nil {
		return IndexStats{}, err
	}

	sessionFiles, err := listSessionFiles(filepath.Join(codexHome, "sessions"))
	if err != nil {
		return IndexStats{}, err
	}
	contentByID := make(map[string]string, len(sessionFiles))
	pathByID := make(map[string]string, len(sessionFiles))
	for _, path := range sessionFiles {
		id := threadIDFromRolloutPath(path)
		if id == "" {
			continue
		}
		text, err := extractSessionText(path, opts.SessionTextBytes)
		if err != nil {
			continue
		}
		contentByID[id] = text
		pathByID[id] = path
	}
	for i := range threads {
		if threads[i].RolloutPath == "" {
			threads[i].RolloutPath = pathByID[threads[i].ID]
		}
		threads[i].Content = contentByID[threads[i].ID]
		threads[i].CodexURL = "codex://threads/" + threads[i].ID
	}

	if err := writeIndex(indexDB, threads); err != nil {
		return IndexStats{}, err
	}
	return IndexStats{
		Threads:       len(threads),
		SessionFiles:  len(sessionFiles),
		IndexedAt:     time.Now(),
		IndexDB:       indexDB,
		StateSnapshot: snapshot,
	}, nil
}

func normalizeCodexHome(path string) (string, error) {
	if path == "" {
		return DefaultCodexHome()
	}
	return ExpandPath(path)
}

func normalizeIndexDB(path string) (string, error) {
	if path == "" {
		return DefaultIndexDB()
	}
	return ExpandPath(path)
}

func backupStateDB(codexHome string) (string, error) {
	source := filepath.Join(codexHome, "state_5.sqlite")
	if _, err := os.Stat(source); err != nil {
		return "", fmt.Errorf("state db not found: %w", err)
	}
	dir, err := os.MkdirTemp("", "codex-history-state-*")
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, "state_5.snapshot.sqlite")
	cmd := exec.Command("sqlite3", source, ".backup '"+strings.ReplaceAll(target, "'", "''")+"'")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("sqlite backup failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return target, nil
}

func loadThreads(snapshot string) ([]Thread, error) {
	db, err := sql.Open("sqlite", snapshot)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		select
			id,
			rollout_path,
			coalesce(created_at_ms, created_at, 0) as created_at,
			coalesce(updated_at_ms, updated_at, 0) as updated_at,
			coalesce(source, '') as source,
			coalesce(cwd, '') as cwd,
			coalesce(title, '') as title,
			coalesce(archived, 0) as archived,
			coalesce(first_user_message, '') as first_user_message,
			coalesce(preview, '') as preview
		from threads
		order by updated_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []Thread
	for rows.Next() {
		var thread Thread
		var createdAt, updatedAt int64
		var archived int
		if err := rows.Scan(
			&thread.ID,
			&thread.RolloutPath,
			&createdAt,
			&updatedAt,
			&thread.Source,
			&thread.CWD,
			&thread.Title,
			&archived,
			&thread.FirstUserMessage,
			&thread.Preview,
		); err != nil {
			return nil, err
		}
		thread.CreatedAt = TimeFromUnixish(createdAt)
		thread.UpdatedAt = TimeFromUnixish(updatedAt)
		thread.Archived = archived != 0
		if thread.Title == "" {
			thread.Title = strings.TrimSpace(thread.FirstUserMessage)
		}
		if thread.Title == "" {
			thread.Title = "(untitled)"
		}
		threads = append(threads, thread)
	}
	return threads, rows.Err()
}

func listSessionFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func threadIDFromRolloutPath(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".jsonl")
	parts := strings.Split(base, "-")
	if len(parts) < 7 {
		return ""
	}
	return strings.Join(parts[len(parts)-5:], "-")
}

func extractSessionText(path string, maxBytes int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 256*1024)
	var builder strings.Builder
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			appendSessionLine(&builder, line, maxBytes)
			if builder.Len() >= maxBytes {
				break
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return builder.String(), nil
		}
	}
	return builder.String(), nil
}

func appendSessionLine(builder *strings.Builder, line []byte, maxBytes int) {
	var value any
	if err := json.Unmarshal(line, &value); err != nil {
		appendLimited(builder, string(line), maxBytes)
		return
	}
	var parts []string
	collectStrings(value, "", &parts)
	if len(parts) == 0 {
		return
	}
	appendLimited(builder, strings.Join(parts, "\n"), maxBytes)
	appendLimited(builder, "\n", maxBytes)
}

func collectStrings(value any, key string, out *[]string) {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			collectStrings(child, k, out)
		}
	case []any:
		for _, child := range v {
			collectStrings(child, key, out)
		}
	case string:
		if shouldIndexStringKey(key, v) {
			*out = append(*out, v)
		}
	}
}

func shouldIndexStringKey(key string, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if len(value) > 50_000 {
		return false
	}
	switch key {
	case "text", "content", "message", "body", "summary", "cmd", "command", "title", "preview", "name", "cwd", "path":
		return true
	default:
		return len(value) >= 8 && !looksLikeOpaqueID(value)
	}
}

func looksLikeOpaqueID(value string) bool {
	if len(value) > 180 {
		return true
	}
	if strings.Count(value, "-") >= 4 && len(value) >= 32 {
		return true
	}
	return false
}

func appendLimited(builder *strings.Builder, value string, maxBytes int) {
	if builder.Len() >= maxBytes {
		return
	}
	remaining := maxBytes - builder.Len()
	if len(value) > remaining {
		value = value[:remaining]
	}
	builder.WriteString(value)
}

func writeIndex(path string, threads []Thread) error {
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec(`
		pragma journal_mode = wal;
		create table threads (
			id text primary key,
			title text not null,
			cwd text not null,
			rollout_path text not null,
			source text not null,
			created_at text not null,
			updated_at text not null,
			created_at_ms integer not null,
			updated_at_ms integer not null,
			archived integer not null,
			first_user_message text not null,
			preview text not null,
			codex_url text not null
		);
		create virtual table thread_fts using fts5(
			id unindexed,
			title,
			cwd,
			first_user_message,
			preview,
			content,
			tokenize = 'trigram'
		);
		create index threads_updated_at_ms_idx on threads(updated_at_ms desc);
		create index threads_cwd_idx on threads(cwd);
		create index threads_archived_idx on threads(archived);
		create table metadata (
			key text primary key,
			value text not null
		);
	`); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	threadStmt, err := tx.Prepare(`
		insert into threads (
			id, title, cwd, rollout_path, source, created_at, updated_at,
			created_at_ms, updated_at_ms, archived, first_user_message, preview, codex_url
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer threadStmt.Close()
	ftsStmt, err := tx.Prepare(`
		insert into thread_fts (
			id, title, cwd, first_user_message, preview, content
		) values (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()

	for _, thread := range threads {
		archived := 0
		if thread.Archived {
			archived = 1
		}
		if _, err := threadStmt.Exec(
			thread.ID,
			thread.Title,
			thread.CWD,
			thread.RolloutPath,
			thread.Source,
			formatTime(thread.CreatedAt),
			formatTime(thread.UpdatedAt),
			thread.CreatedAt.UnixMilli(),
			thread.UpdatedAt.UnixMilli(),
			archived,
			thread.FirstUserMessage,
			thread.Preview,
			thread.CodexURL,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := ftsStmt.Exec(
			thread.ID,
			thread.Title,
			thread.CWD,
			thread.FirstUserMessage,
			thread.Preview,
			thread.Content,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err := tx.Exec(`insert into metadata(key, value) values ('indexed_at', ?)`, time.Now().Format(time.RFC3339)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	_ = os.Remove(tmp + "-wal")
	_ = os.Remove(tmp + "-shm")
	return nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
