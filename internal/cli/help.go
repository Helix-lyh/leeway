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
  leeway login opencode-go work --email you@example.com --key oc_sk-...
  leeway login grok --cookie
  leeway login grok --manual "a=b; c=d"
  leeway login grok home --file ~/.grok/auth.json

cursor / grok / codex / kimi 走官方 CLI 登录（PATH 里要有 cursor-agent / grok / codex / kimi）。
opencode-go / commandcode / deepseek 打开申请页，粘贴 API key（不回显）。
--email 记下账号归属：同一家挂多个订阅时，界面和 status 靠它区分。
--email 只对粘贴 key 的家生效；cursor / grok / codex / kimi 的邮箱从官方会话里读。
cursor 不写 ACCOUNT 时用邮箱当账号名。
--file 把官方会话快照进隔离目录（grok / kimi / cursor / codex），不绑正在用的官方文件。
grok 的 --file 必须是文件里已有 auth_mode / user_id 的完整 auth.json（不要靠设备码 JWT 冒充）。每个号网页授权一次，之后 use 只换文件。

--cookie 从浏览器导入 cookie（仅 macOS）。Linux 用 --manual。
--browser / --profile / --force 是导入时用的。
--manual 手动粘贴 Cookie header。--key 直接写入 API key。
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

列出已保存的号，不访问网上的 provider。台账空着、有号没登录或有家缺登录时退出 1。
`

const useHelp = `leeway use — 把某个号变成本机正在用的那个

  leeway use cursor work
  leeway use grok home
  leeway use kimi work
  leeway use opencode-go work
  leeway use cursor work --json

cursor：往本机所有官方 Cursor 凭据位置写：cursor-agent 的 keychain 项 cursor-access-token（macOS）、
  Cursor IDE 的 state.vscdb、~/.cursor/auth.json；哪一个不存在就跳过，一个都没找到才报错（不再新建 auth.json）。
  用的哪几个会打在 targets 里。Cursor 在跑就先退出（cursor-agent 不算）。
  LEEWAY_CURSOR_KEYCHAIN=0 关掉 keychain 写入（其它值是 keychain 服务名）；LEEWAY_CURSOR_STATE_DB /
  LEEWAY_CURSOR_AUTH_JSON 各自把对应文件写到别处，指定任一个就不再碰真 keychain。
grok：把完整官方会话合并进 ~/.grok/auth.json（瘦设备码文件会拒绝，以免逼 grok 重新网页登录）。
kimi：有隔离会话则写官方 kimi-code.json（尊重 KIMI_CODE_HOME）；只有 key 的号写 DSH。
codex：不写官方 ~/.codex（避免 refresh 把官方会话作废）。
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
