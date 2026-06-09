# codex-history

本项目是本机 Codex 历史只读索引工具。

## 边界

- 只能只读访问 `~/.codex/state_5.sqlite`、`~/.codex/session_index.jsonl`、`~/.codex/sessions/**` 等 Codex 原始数据。
- 禁止写回、修补或迁移 Codex App 的内部状态库。
- 禁止 patch `/Applications/Codex.app` 或修改 `app.asar`。
- 生成索引必须写入独立目录，默认 `~/.codex-history/index.sqlite`。
- Web 服务默认只能监听 `127.0.0.1`，不要暴露到公网。

## 验证

```bash
go test ./...
go run ./cmd/codex-history index
go run ./cmd/codex-history search "关键词" --limit 5
```
