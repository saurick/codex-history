# codex-history

本机 Codex 会话历史只读索引工具。

## 目标

- 只读扫描 `~/.codex/state_5.sqlite` 和 `~/.codex/sessions/**/*.jsonl`
- 生成独立索引库 `~/.codex-history/index.sqlite`
- 支持命令行搜索和本地 Web 查看
- 不写回 Codex App 的内部状态库
- 不 patch Codex App

## 快速开始

```bash
go run ./cmd/codex-history index
go run ./cmd/codex-history search "登录"
make dev_restart
```

默认 Web 地址：

```text
http://127.0.0.1:8787
```

## 命令

### index

```bash
codex-history index
```

常用参数：

```bash
codex-history index \
  --codex-home ~/.codex \
  --index-db ~/.codex-history/index.sqlite
```

索引时会先用 `sqlite3 .backup` 创建 `state_5.sqlite` 快照，再读取快照，避免直接读取 Codex App 正在写入的 live SQLite。

### search

```bash
codex-history search "关键词"
```

常用参数：

```bash
codex-history search "关键词" --project plush-toy-erp --since 30d --limit 50
codex-history search "019eab70" --all
codex-history search "部署" --json
```

### serve

```bash
codex-history serve
```

常用参数：

```bash
codex-history serve --addr 127.0.0.1:8787
```

Web 页面里：

- 点击结果标题或“查看内容”进入本地详情页。详情页分为“对话”和“调试事件”两个 tab。
- “对话”只展示用户和助手消息；“调试事件”展示 developer 指令、工具调用和工具输出。
- 点击“打开 Codex”会走 `codex://threads/<thread-id>` 深链，回到 Codex App。
- 搜索结果默认每页显示 80 条，底部“加载更多”继续分页读取。

索引默认只写入用户和助手消息。developer/system 指令、工具调用参数、工具输出不参与搜索，避免运行时上下文污染会话搜索结果。

### 本地开发服务

推荐用 Makefile 管理本地开发服务。和其他项目一样，`make dev_restart` 会前台常驻并直接输出日志：

```bash
make dev_restart
```

需要后台启动时使用：

```bash
make dev_restart_bg
```

常用命令：

```bash
make dev_status
make dev_stop
```

默认监听 `127.0.0.1:8787`。后台模式会把 PID 写入 `.tmp/codex-history.pid`，日志写入 `.tmp/codex-history.log`。

## 数据边界

`codex-history` 的索引库可能包含本机 Codex 历史里的命令、路径、输出片段和用户消息。默认只监听 `127.0.0.1`，不要把 Web 服务暴露到公网。
