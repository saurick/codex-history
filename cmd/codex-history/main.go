package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/saurick/codex-history/internal/codexhistory"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}
	switch args[0] {
	case "index":
		return runIndex(args[1:])
	case "search":
		return runSearch(args[1:])
	case "serve":
		return runServe(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runIndex(args []string) error {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	codexHome := fs.String("codex-home", "", "Codex home directory, defaults to CODEX_HOME or ~/.codex")
	indexDB := fs.String("index-db", "", "index database path, defaults to ~/.codex-history/index.sqlite")
	sessionTextBytes := fs.Int("session-text-bytes", 300_000, "maximum indexed text bytes per session")
	if err := fs.Parse(args); err != nil {
		return err
	}
	stats, err := codexhistory.BuildIndex(codexhistory.IndexOptions{
		CodexHome:        *codexHome,
		IndexDB:          *indexDB,
		SessionTextBytes: *sessionTextBytes,
	})
	if err != nil {
		return err
	}
	fmt.Printf("indexed %d threads from %d session files\n", stats.Threads, stats.SessionFiles)
	fmt.Printf("index db: %s\n", stats.IndexDB)
	return nil
}

func runSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	indexDB := fs.String("index-db", "", "index database path, defaults to ~/.codex-history/index.sqlite")
	project := fs.String("project", "", "filter cwd containing this text")
	sinceText := fs.String("since", "", "filter updated time, such as 30d or 2026-06-01")
	includeArchived := fs.Bool("all", false, "include archived threads")
	limit := fs.Int("limit", 20, "maximum results")
	jsonOutput := fs.Bool("json", false, "print JSON")
	flagArgs, queryArgs := splitSearchArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	query := strings.Join(queryArgs, " ")
	since, err := codexhistory.ParseSince(*sinceText, time.Now())
	if err != nil {
		return err
	}
	results, err := codexhistory.Search(*indexDB, codexhistory.SearchOptions{
		Query:           query,
		ProjectContains: *project,
		Since:           since,
		IncludeArchived: *includeArchived,
		Limit:           *limit,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)
	}
	printResults(results)
	return nil
}

func splitSearchArgs(args []string) ([]string, []string) {
	valueFlags := map[string]bool{
		"-index-db": true, "--index-db": true,
		"-project": true, "--project": true,
		"-since": true, "--since": true,
		"-limit": true, "--limit": true,
	}
	boolFlags := map[string]bool{
		"-all": true, "--all": true,
		"-json": true, "--json": true,
	}
	var flagArgs []string
	var queryArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
			name := strings.SplitN(arg, "=", 2)[0]
			if valueFlags[name] || boolFlags[name] {
				flagArgs = append(flagArgs, arg)
				continue
			}
		}
		if boolFlags[arg] {
			flagArgs = append(flagArgs, arg)
			continue
		}
		if valueFlags[arg] {
			flagArgs = append(flagArgs, arg)
			if i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		queryArgs = append(queryArgs, arg)
	}
	return flagArgs, queryArgs
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	indexDB := fs.String("index-db", "", "index database path, defaults to ~/.codex-history/index.sqlite")
	addr := fs.String("addr", "127.0.0.1:8787", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return codexhistory.Serve(codexhistory.ServerOptions{Addr: *addr, IndexDB: *indexDB})
}

func printResults(results []codexhistory.SearchResult) {
	if len(results) == 0 {
		fmt.Println("no results")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "UPDATED\tTITLE\tCWD\tTHREAD")
	for _, result := range results {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\n",
			result.UpdatedAt.Local().Format("2006-01-02 15:04"),
			oneLine(result.Title, 42),
			oneLine(result.CWD, 46),
			result.CodexURL,
		)
	}
	_ = w.Flush()
	for _, result := range results {
		if result.Snippet == "" {
			continue
		}
		fmt.Printf("\n%s\n  %s\n  %s\n", result.ID, result.Snippet, result.RolloutPath)
	}
}

func oneLine(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max-1]) + "…"
}

func printUsage() {
	fmt.Print(`codex-history indexes and searches local Codex threads.

Usage:
  codex-history index [--codex-home ~/.codex] [--index-db ~/.codex-history/index.sqlite]
  codex-history search [flags] <query>
  codex-history serve [--addr 127.0.0.1:8787]

Examples:
  codex-history index
  codex-history search "登录" --since 30d --limit 50
  codex-history search "plush-toy-erp" --project plush-toy-erp
  codex-history serve
`)
}
