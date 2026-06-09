package codexhistory

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func Search(indexDB string, opts SearchOptions) ([]SearchResult, error) {
	indexDB, err := normalizeIndexDB(indexDB)
	if err != nil {
		return nil, err
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	db, err := sql.Open("sqlite", indexDB)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.Exec(`pragma busy_timeout = 5000`); err != nil {
		return nil, err
	}

	query := strings.TrimSpace(opts.Query)
	like := "%" + escapeLike(query) + "%"

	var where []string
	var args []any
	if !opts.IncludeArchived {
		where = append(where, "t.archived = 0")
	}
	if !opts.Since.IsZero() {
		where = append(where, "t.updated_at_ms >= ?")
		args = append(args, opts.Since.UnixMilli())
	}
	if opts.ProjectContains != "" {
		where = append(where, "t.cwd like ? escape '\\'")
		args = append(args, "%"+escapeLike(opts.ProjectContains)+"%")
	}
	var sqlText string
	if query != "" {
		sqlText = `
		with matched_ids as (
			select id from threads
			where id = ?
				or title like ? escape '\'
				or cwd like ? escape '\'
				or first_user_message like ? escape '\'
			union
			select id from thread_fts
			where thread_fts match ?
		)
		select
			t.id, t.title, t.cwd, t.rollout_path, t.source,
			t.created_at, t.updated_at, t.created_at_ms, t.updated_at_ms,
			t.archived, t.first_user_message, t.preview, t.codex_url,
			coalesce((
				select f.content from thread_fts f where f.id = t.id limit 1
			), '') as content
		from threads t
		where t.id in (select id from matched_ids)`
		args = append([]any{query, like, like, like, quoteFTS(query)}, args...)
	} else {
		sqlText = `
		select
			t.id, t.title, t.cwd, t.rollout_path, t.source,
			t.created_at, t.updated_at, t.created_at_ms, t.updated_at_ms,
			t.archived, t.first_user_message, t.preview, t.codex_url,
			coalesce((
				select f.content from thread_fts f where f.id = t.id limit 1
			), '') as content
		from threads t`
	}
	if len(where) > 0 {
		if query != "" {
			sqlText += "\nand " + strings.Join(where, "\nand ")
		} else {
			sqlText += "\nwhere " + strings.Join(where, "\nand ")
		}
	}
	sqlText += "\norder by t.updated_at_ms desc\nlimit ? offset ?"
	args = append(args, opts.Limit, opts.Offset)

	rows, err := db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]SearchResult, 0)
	for rows.Next() {
		var result SearchResult
		var createdAt, updatedAt int64
		var archived int
		var createdAtText, updatedAtText string
		if err := rows.Scan(
			&result.ID,
			&result.Title,
			&result.CWD,
			&result.RolloutPath,
			&result.Source,
			&createdAtText,
			&updatedAtText,
			&createdAt,
			&updatedAt,
			&archived,
			&result.FirstUserMessage,
			&result.Preview,
			&result.CodexURL,
			&result.Content,
		); err != nil {
			return nil, err
		}
		result.CreatedAt = TimeFromUnixish(createdAt)
		result.UpdatedAt = TimeFromUnixish(updatedAt)
		result.Archived = archived != 0
		result.Snippet = makeSnippet(query, result)
		results = append(results, result)
		_ = createdAtText
		_ = updatedAtText
	}
	return results, rows.Err()
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func quoteFTS(value string) string {
	value = strings.ReplaceAll(value, `"`, `""`)
	return `"` + value + `"`
}

func makeSnippet(query string, result SearchResult) string {
	query = strings.TrimSpace(query)
	fields := []string{result.Title, result.FirstUserMessage, result.Content}
	if query == "" {
		for _, field := range fields {
			if s := compact(field); s != "" {
				return truncateRunes(s, 180)
			}
		}
		return ""
	}
	lowerQuery := strings.ToLower(query)
	for _, field := range fields {
		lower := strings.ToLower(field)
		idx := strings.Index(lower, lowerQuery)
		if idx >= 0 {
			start := idx - 70
			if start < 0 {
				start = 0
			}
			end := idx + len(query) + 110
			if end > len(field) {
				end = len(field)
			}
			return truncateRunes(compact(field[start:end]), 220)
		}
	}
	return truncateRunes(compact(result.Content), 180)
}

func compact(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

func Count(indexDB string) (int, error) {
	indexDB, err := normalizeIndexDB(indexDB)
	if err != nil {
		return 0, err
	}
	db, err := sql.Open("sqlite", indexDB)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	if _, err := db.Exec(`pragma busy_timeout = 5000`); err != nil {
		return 0, err
	}
	var count int
	if err := db.QueryRow("select count(*) from threads").Scan(&count); err != nil {
		return 0, fmt.Errorf("index db not ready, run index first: %w", err)
	}
	return count, nil
}
