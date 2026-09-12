package cli

import (
	"fmt"
	"os"
	"strings"
)

const rootHelp = `leeway — 查各家 AI 编程额度还剩多少

  usage  [PROVIDER [ACCOUNT]]   [--json] [--watch N] [--last N]
  login  PROVIDER [ACCOUNT]     [--cookie] [--manual] [--file] [--key]
                                [--browser] [--profile] [--force]
                                [--no-browser] [--timeout N]
  logout PROVIDER [ACCOUNT]
  status                        [--json]
  use    PROVIDER ACCOUNT       [--json]
  help   [COMMAND]

PROVIDER / ACCOUNT 也可写成 --provider / --account（tag、邮箱或订阅名）。
不写 ACCOUNT 不读隐式 default（官方 keychain / 共享 ~/.grok）。
某一条的参数：leeway help usage（还有 login / logout / status / use）。
login cursor/grok/codex/kimi 走官方 CLI；其余粘贴 key。

退出码  0 正常  1 有失败或未配置  2 用法错
`

const usageHelp = `leeway usage — 查额度

  leeway usage
  leeway usage cursor
  leeway usage cursor work
  leeway usage --json
  leeway usage --watch 30
  leeway usage --last 12

PROVIDER / ACCOUNT 过滤要看的家和号。不写 ACCOUNT 只列出已命名账号，不读隐式 default。
表上默认画相对该窗口上次成功探测的增量（↑2 / +$12 / reset）。JSON 始终带可选 delta。
--last N 看近 N 次的点阵，不联网。不能和 --watch 一起用。
`

const loginHelp = `leeway login — 给一家登录或写入凭据

  leeway login cursor
  leeway login grok home
  leeway login opencode-go work
  leeway login commandcode goat
  leeway login deepseek --key sk-...
  leeway login grok --cookie
  leeway login grok --manual "a=b; c=d"
  leeway login grok home --file ~/.grok/auth.json

cursor / grok 走官方 CLI 浏览器登录（PATH 里要有 cursor-agent / grok）。
opencode-go / commandcode / deepseek 打开申请页，粘贴 API key（不回显）。
cursor 不写 ACCOUNT 时用邮箱当账号名。

--cookie 从浏览器导入 cookie（仅 macOS）。Linux 用 --manual。
--browser / --profile / --force 是导入时用的。
--manual 手动粘贴 Cookie header。--file 指定会话文件。--key 直接写入 API key。
--no-browser 只打印登录 URL。--timeout 是等浏览器授权的分钟数（默认 15）。
`

const logoutHelp = `leeway logout — 清掉 leeway 保存的登录

  leeway logout cursor
  leeway logout grok home

只删 leeway 自己的目录，不动官方 CLI / keychain。
`

const statusHelp = `leeway status — 各家登录了没、号还在不在

  leeway status
  leeway status --json

列出已保存的号，不访问网上的 provider。
`

const useHelp = `leeway use — 把某个号变成本机正在用的那个

  leeway use cursor work
  leeway use grok home
  leeway use opencode-go work
  leeway use cursor work --json

cursor：写入 Cursor IDE（state.vscdb）和 ~/.cursor/auth.json。Cursor 在跑就先退出。
grok：合并进 ~/.grok/auth.json。
其余：把 API key 写给 DSH。

这不是「leeway 接下来查哪号」。查额度用 leeway usage cursor work。
macOS / Linux。
`

var helpTopics = []struct {
	name string
	text string
}{
	{"usage", usageHelp},
	{"login", loginHelp},
	{"logout", logoutHelp},
	{"status", statusHelp},
	{"use", useHelp},
}

func helpTopicNames() string {
	names := make([]string, len(helpTopics))
	for i, t := range helpTopics {
		names[i] = t.name
	}
	return strings.Join(names, " / ")
}

func isHelpArg(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

func printHelp(w *os.File, text string) {
	fmt.Fprint(w, strings.TrimRight(text, "\n")+"\n")
}

func helpTopic(name string) (string, bool) {
	for _, t := range helpTopics {
		if t.name == name {
			return t.text, true
		}
	}
	return "", false
}

func runHelpTopic(name string) int {
	text, ok := helpTopic(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "没有 %q 这条命令的帮助。可用：%s\n", name, helpTopicNames())
		return 2
	}
	printHelp(os.Stdout, text)
	return 0
}
