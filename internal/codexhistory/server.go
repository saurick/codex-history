package codexhistory

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"
)

type ServerOptions struct {
	Addr    string
	IndexDB string
}

func Serve(opts ServerOptions) error {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:8787"
	}
	indexDB, err := normalizeIndexDB(opts.IndexDB)
	if err != nil {
		return err
	}
	if _, err := Count(indexDB); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pageTemplate.Execute(w, map[string]string{"IndexDB": indexDB})
	})
	mux.HandleFunc("/thread", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		detail, err := GetThreadDetail(indexDB, id, 120_000)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = detailTemplate.Execute(w, struct {
			ThreadDetail
			SafeCodexURL template.URL
		}{
			ThreadDetail: detail,
			SafeCodexURL: template.URL(detail.CodexURL),
		})
	})
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		opts, err := searchOptionsFromRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		results, err := Search(indexDB, opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results":  results,
			"limit":    opts.Limit,
			"offset":   opts.Offset,
			"has_more": len(results) == opts.Limit,
		})
	})
	mux.HandleFunc("/api/thread", func(w http.ResponseWriter, r *http.Request) {
		detail, err := GetThreadDetail(indexDB, r.URL.Query().Get("id"), 120_000)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(detail)
	})

	log.Printf("codex-history serving http://%s", opts.Addr)
	return http.ListenAndServe(opts.Addr, mux)
}

func searchOptionsFromRequest(r *http.Request) (SearchOptions, error) {
	q := r.URL.Query()
	limit := 50
	if value := q.Get("limit"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 || n > 200 {
			return SearchOptions{}, fmt.Errorf("limit must be 1-200")
		}
		limit = n
	}
	offset := 0
	if value := q.Get("offset"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return SearchOptions{}, fmt.Errorf("offset must be >= 0")
		}
		offset = n
	}
	since, err := ParseSince(q.Get("since"), time.Now())
	if err != nil {
		return SearchOptions{}, err
	}
	return SearchOptions{
		Query:           q.Get("q"),
		ProjectContains: q.Get("project"),
		Since:           since,
		IncludeArchived: q.Get("archived") == "1",
		Limit:           limit,
		Offset:          offset,
	}, nil
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>codex-history</title>
  <style>
    :root { color-scheme: light dark; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: Canvas; color: CanvasText; }
    header { position: sticky; top: 0; z-index: 1; padding: 16px 20px; border-bottom: 1px solid color-mix(in srgb, CanvasText 14%, transparent); background: color-mix(in srgb, Canvas 94%, transparent); backdrop-filter: blur(12px); }
    h1 { margin: 0 0 12px; font-size: 18px; font-weight: 650; }
    form { display: grid; grid-template-columns: minmax(220px, 1fr) 180px 110px 92px auto; gap: 8px; align-items: center; }
    input, select, button { height: 34px; border: 1px solid color-mix(in srgb, CanvasText 18%, transparent); border-radius: 6px; padding: 0 10px; background: Canvas; color: CanvasText; font: inherit; }
    button { cursor: pointer; background: color-mix(in srgb, CanvasText 8%, Canvas); }
    main { padding: 14px 20px 40px; }
    .meta { margin: 0 0 12px; color: color-mix(in srgb, CanvasText 62%, transparent); font-size: 13px; }
    .result { padding: 14px 0; border-bottom: 1px solid color-mix(in srgb, CanvasText 10%, transparent); }
    .title { display: flex; gap: 8px; align-items: baseline; margin-bottom: 6px; }
    .title a { color: LinkText; text-decoration: none; font-weight: 650; }
    .title a:hover { text-decoration: underline; }
    .time { white-space: nowrap; color: color-mix(in srgb, CanvasText 58%, transparent); font-size: 12px; }
    .cwd, .path, .id { color: color-mix(in srgb, CanvasText 58%, transparent); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; overflow-wrap: anywhere; }
    .snippet { margin-top: 8px; line-height: 1.45; font-size: 14px; overflow-wrap: anywhere; }
    .links { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 8px; font-size: 12px; }
    .links a { color: LinkText; }
    .empty, .error { padding: 24px 0; color: color-mix(in srgb, CanvasText 62%, transparent); }
    .actions { padding: 18px 0; }
    .load-more { min-width: 140px; }
    @media (max-width: 760px) {
      form { grid-template-columns: 1fr; }
      input, select, button { width: 100%; box-sizing: border-box; }
      .title { display: block; }
    }
  </style>
</head>
<body>
  <header>
    <h1>codex-history</h1>
    <form id="searchForm">
      <input id="q" name="q" autofocus placeholder="搜索标题、项目、消息内容、thread id">
      <input id="project" name="project" placeholder="项目路径包含">
      <select id="since" name="since">
        <option value="">全部时间</option>
        <option value="7d">最近 7 天</option>
        <option value="30d" selected>最近 30 天</option>
        <option value="90d">最近 90 天</option>
      </select>
      <select id="archived" name="archived">
        <option value="0">未归档</option>
        <option value="1">含归档</option>
      </select>
      <button type="submit">搜索</button>
    </form>
  </header>
  <main>
    <p class="meta">索引库：{{.IndexDB}}</p>
    <div id="status" class="meta"></div>
    <div id="results"></div>
  </main>
  <script>
    const form = document.getElementById('searchForm');
    const results = document.getElementById('results');
    const status = document.getElementById('status');
    const pageSize = 80;
    let loaded = 0;
    let lastParams = null;

    form.addEventListener('submit', (event) => {
      event.preventDefault();
      runSearch();
    });

    async function runSearch(append = false) {
      const params = new URLSearchParams(new FormData(form));
      params.set('limit', String(pageSize));
      params.set('offset', append ? String(loaded) : '0');
      if (!append) {
        loaded = 0;
        results.innerHTML = '';
      }
      lastParams = params;
      status.textContent = append ? '加载中...' : '搜索中...';
      try {
        const response = await fetch('/api/search?' + params.toString());
        if (!response.ok) throw new Error(await response.text());
        const data = await response.json();
        renderResults(data.results || [], Boolean(data.has_more), append);
      } catch (error) {
        status.textContent = '';
        results.innerHTML = '<div class="error">' + escapeHtml(String(error.message || error)) + '</div>';
      }
    }

    function renderResults(items, hasMore, append) {
      loaded += items.length;
      status.textContent = '已显示 ' + loaded + ' 条' + (hasMore ? '，还有更多' : '');
      if (!append && !items.length) {
        results.innerHTML = '<div class="empty">没有匹配结果</div>';
        return;
      }
      const html = items.map(item => {
	        const title = escapeHtml(item.title || '(untitled)');
	        const cwd = escapeHtml(item.cwd || '');
	        const path = escapeHtml(item.rollout_path || '');
	        const snippet = escapeHtml(item.snippet || '');
	        const id = escapeHtml(item.id || '');
	        const updated = escapeHtml(item.updated_at ? new Date(item.updated_at).toLocaleString() : '');
	        const codexURL = escapeAttr(item.codex_url || ('codex://threads/' + item.id));
	        const detailURL = '/thread?id=' + encodeURIComponent(item.id || '');
	        return '<article class="result">' +
	          '<div class="title"><a href="' + detailURL + '">' + title + '</a> <span class="time">' + updated + '</span></div>' +
	          '<div class="cwd">' + cwd + '</div>' +
	          '<div class="snippet">' + snippet + '</div>' +
	          '<div class="links">' +
	            '<a href="' + detailURL + '">查看内容</a>' +
	            '<a href="' + codexURL + '">打开 Codex</a>' +
	            '<span class="id">' + id + '</span>' +
	          '</div>' +
	          '<div class="path">' + path + '</div>' +
	        '</article>';
	      }).join('');
      const more = hasMore ? '<div class="actions"><button class="load-more" type="button" onclick="runSearch(true)">加载更多</button></div>' : '';
      if (append) {
        const oldButton = results.querySelector('.actions');
        if (oldButton) oldButton.remove();
        results.insertAdjacentHTML('beforeend', html + more);
      } else {
        results.innerHTML = html + more;
      }
	    }

    function escapeHtml(value) {
      return value.replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
    }
	    function escapeAttr(value) {
	      return escapeHtml(value);
	    }
    runSearch();
  </script>
</body>
</html>`))

var detailTemplate = template.Must(template.New("detail").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} - codex-history</title>
  <style>
    :root { color-scheme: light dark; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: Canvas; color: CanvasText; }
    header { position: sticky; top: 0; z-index: 1; padding: 16px 20px; border-bottom: 1px solid color-mix(in srgb, CanvasText 14%, transparent); background: color-mix(in srgb, Canvas 94%, transparent); backdrop-filter: blur(12px); }
    main { padding: 14px 20px 48px; max-width: 1080px; }
    h1 { margin: 0 0 10px; font-size: 20px; line-height: 1.35; }
    a { color: LinkText; }
    .nav { display: flex; flex-wrap: wrap; gap: 12px; margin-bottom: 10px; font-size: 13px; }
    .meta { color: color-mix(in srgb, CanvasText 60%, transparent); font-size: 12px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; overflow-wrap: anywhere; }
    .items { margin-top: 16px; display: grid; gap: 14px; }
    .item { border: 1px solid color-mix(in srgb, CanvasText 12%, transparent); border-radius: 8px; padding: 12px; background: color-mix(in srgb, CanvasText 3%, Canvas); }
    .item-header { display: flex; flex-wrap: wrap; gap: 8px; align-items: baseline; margin-bottom: 8px; }
    .badge { border: 1px solid color-mix(in srgb, CanvasText 18%, transparent); border-radius: 999px; padding: 2px 8px; font-size: 12px; color: color-mix(in srgb, CanvasText 70%, transparent); }
    .message.user .badge { color: #0a7f42; }
    .message.assistant .badge { color: #5b5fc7; }
    .tool .badge, .tool-output .badge { color: #a45c00; }
    .timestamp { color: color-mix(in srgb, CanvasText 54%, transparent); font-size: 12px; }
    pre { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.5; font: 13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; }
    .truncated { margin-top: 8px; color: color-mix(in srgb, CanvasText 58%, transparent); font-size: 12px; }
    .empty { padding: 28px 0; color: color-mix(in srgb, CanvasText 62%, transparent); }
  </style>
</head>
<body>
  <header>
    <nav class="nav">
      <a href="/">返回搜索</a>
      <a href="{{.SafeCodexURL}}">打开 Codex</a>
    </nav>
    <h1>{{.Title}}</h1>
    <div class="meta">{{.ID}}</div>
    <div class="meta">{{.CWD}}</div>
    <div class="meta">{{.RolloutPath}}</div>
    <div class="meta">updated: {{if not .UpdatedAt.IsZero}}{{.UpdatedAt.Format "2006-01-02 15:04:05"}}{{end}}</div>
  </header>
  <main>
    {{if .Items}}
      <section class="items">
        {{range .Items}}
          <article class="item {{.Kind}} {{.Role}}">
            <div class="item-header">
              <span class="badge">{{.Title}}</span>
              <span class="timestamp">{{.Timestamp}}</span>
            </div>
            <pre>{{.Text}}</pre>
            {{if .Truncated}}<div class="truncated">这条内容太长，网页视图已截断；需要逐字核对时请查看上方原始 JSONL 文件。</div>{{end}}
          </article>
        {{end}}
      </section>
    {{else}}
      <div class="empty">这个会话没有可展示的消息项；可以查看上方原始 JSONL 路径。</div>
    {{end}}
  </main>
</body>
</html>`))
