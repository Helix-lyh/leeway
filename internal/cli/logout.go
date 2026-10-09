package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"leeway/internal/auth"
	"leeway/internal/config"
)

func runLogout(args []string) int {
	var providerID, account string
	fs := newFlagSet("logout", nil)
	fs.Help(logoutHelp)
	fs.providerAccount(&providerID, &account)
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" {
		fs.Usage()
		return 2
	}
	if !knownProvider(providerID) {
		fmt.Fprintf(os.Stderr, "不认识的服务商 %q\n", providerID)
		fs.Usage()
		return 2
	}

	_ = auth.ClearCookieCache(providerID, "")

	switch providerID {
	case "cursor":
		return logoutCursor(account)
	case "grok":
		return logoutGrok(account)
	case "codex", "kimi":
		return logoutIsolated(providerID, account)
	default:
		return logoutAPIKey(providerID, account)
	}
}

func logoutIsolated(providerID, account string) int {
	home, err := isolatedSessionHome(providerID, account)
	if err != nil {
		fmt.Fprintln(os.Stderr, providerID+" home:", err)
		return 1
	}
	root, err := auth.SessionDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "session dir:", err)
		return 1
	}
	if !underDir(root, home) {
		fmt.Fprintf(os.Stderr, "不删除 %s：它不是 leeway 的登录目录\n", home)
		return 1
	}
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintln(os.Stderr, "clear "+providerID+" auth:", err)
		return 1
	}

	name := account
	if name == "" || name == "default" {
		name = "default"
	}
	if cfg, err := config.Load(); err == nil {
		_ = cfg.RemoveAccount(providerID, name)
	}
	fmt.Fprintf(os.Stderr, "已删除 %s 账号 %s 的登录（%s）\n", providerID, displayAccount(name), home)
	return 0
}

func logoutAPIKey(providerID, account string) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	name := account
	if name == "" {
		name = "default"
	}
	if err := cfg.RemoveAccount(providerID, name); err != nil {
		fmt.Fprintln(os.Stderr, "config save:", err)
		return 1
	}
	if account == "" || account == "default" {
		if cfg.APIKeys != nil {
			delete(cfg.APIKeys, providerID)
			if err := cfg.Save(); err != nil {
				fmt.Fprintln(os.Stderr, "config save:", err)
				return 1
			}
		}
	}
	fmt.Fprintf(os.Stderr, "已删除 %s 账号 %s\n", providerID, displayAccount(account))
	return 0
}

func underDir(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if root == "" || root == string(os.PathSeparator) {
		return false
	}
	if path == root {
		return true
	}
	prefix := root + string(os.PathSeparator)
	return strings.HasPrefix(path, prefix)
}
