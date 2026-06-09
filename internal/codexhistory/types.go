package codexhistory

import "time"

type Thread struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	CWD              string    `json:"cwd"`
	RolloutPath      string    `json:"rollout_path"`
	Source           string    `json:"source"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Archived         bool      `json:"archived"`
	FirstUserMessage string    `json:"first_user_message"`
	Preview          string    `json:"preview"`
	Content          string    `json:"-"`
	CodexURL         string    `json:"codex_url"`
}

type SearchOptions struct {
	Query           string
	ProjectContains string
	Since           time.Time
	IncludeArchived bool
	Limit           int
	Offset          int
}

type SearchResult struct {
	Thread
	Snippet string `json:"snippet"`
}

type ThreadDetail struct {
	Thread
	Items []ThreadItem `json:"items"`
}

type ThreadItem struct {
	Timestamp string `json:"timestamp"`
	Kind      string `json:"kind"`
	Role      string `json:"role"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}
