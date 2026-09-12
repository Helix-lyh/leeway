# leeway（余量·裕度）

[![License: MIT](https://img.shields.io/badge/license-MIT-6e5aff?style=flat-square)](LICENSE)

查 AI 编程额度还剩多少：用量、余额、何时重置。纯 CLI，JSON 优先，stdout 只出数据。

支持：**deepseek / kimi / grok / codex / cursor / opencode-go / commandcode**。

复刻 [CodexBar](https://github.com/steipete/CodexBar) 的探测核心，没有菜单栏或后台服务。

## 安装

```sh
go install ./cmd/leeway
# 或
go build -o leeway ./cmd/leeway
```

## 用法

```sh
leeway help                          # 命令树
leeway help login                    # 某一条的参数
leeway usage                         # 已配置的订阅
leeway usage cursor work
leeway usage --json                  # 给脚本 / agent
leeway usage --last 24               # 近 24 次点阵
leeway usage --watch 30

leeway status                        # 谁登了、缺啥
leeway login cursor                  # 账号名默认用邮箱；再加 work 是 tag
leeway login grok home
leeway login opencode-go work
leeway login codex                   # 走官方 CLI 的浏览器登录
leeway login kimi
leeway logout cursor work
```

`leeway login cursor/grok/codex/kimi` 调官方 CLI；其余粘贴 API key。cookie 备选：`leeway login kimi --cookie`、`leeway login grok --manual`。近 N 次点阵用 `leeway usage --last N`。

某一条看 `leeway help login`（还有 usage / logout / status / use）。

`PROVIDER` / `ACCOUNT` 也可写成 `--provider` / `--account`。ACCOUNT 是 tag、邮箱（cursor）或订阅名（opencode-go / commandcode）。不写时不读官方 keychain / 共享 `~/.grok`；要看那个号请加 `default`。

`leeway use` 把某个号变成本机正在用的那个，不是切 leeway 自己查哪号：

```sh
leeway use cursor work               # 写入 Cursor IDE + ~/.cursor/auth.json（先退出 Cursor）
leeway use grok home                 # 合并进 ~/.grok/auth.json
leeway use opencode-go work          # 把 key 写给 DSH
```

macOS / Linux。Cursor 必须先退出。`leeway use cursor` 写 `state.vscdb`：

- macOS: `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`
- Linux: `~/.config/Cursor/...`（Flatpak 回退已在 switcher 里）

Grok 官方文件两边都是 `~/.grok/auth.json`。先 `leeway login` 把要用的号都收进 leeway，再 `use`。

## 各家怎么登录

| Provider | 登录 |
|---|---|
| cursor | `leeway login cursor` |
| grok | `leeway login grok` |
| opencode-go / commandcode / deepseek | `leeway login …`（粘贴 API key） |
| codex / kimi | `leeway login codex` / `leeway login kimi` |
| grok / kimi 备选 | `leeway login grok --manual` / `leeway login kimi --cookie` |

## 配置

- 配置：`~/.config/leeway/config.json`（0600，`LEEWAY_CONFIG` 可覆盖）
- 缓存：`~/.cache/leeway/`（`LEEWAY_CACHE`）
- 凭据：**环境变量 → 配置文件**。cookie 只在你显式导入或 `--manual` 时写入缓存。

```json
{
  "apiKeys": { "deepseek": "sk-..." },
  "providers": {
    "opencode-go": {
      "accounts": [
        { "name": "work", "apiKey": "sk-..." },
        { "name": "personal", "apiKey": "sk-..." }
      ]
    }
  }
}
```

## 给脚本用

- stdout 只有表格或 JSON；日志和错误走 stderr
- `leeway usage --json`：`providers[].accounts[].snapshot.windows[]`（`usedPercent` / `used` / `limit` / `resetsAt`，可选 `delta`）
- `leeway usage --last N --json`：近 N 次按窗口的时间序列（`series[].points[]`）；`history` 是这条的别名
- 退出码：`0` 全部正常；`1` 有未配置或失败；`2` 用法错误
- 超时默认 15s（`LEEWAY_TIMEOUT`），无 TTY 自动纯文本，`COLOR=0` 关颜色

## 文档

| | |
|---|---|
| [docs/DESIGN.md](docs/DESIGN.md) | 架构与数据模型 |
| [docs/PROVIDER-FETCH.md](docs/PROVIDER-FETCH.md) | 各家端点与凭据 |
| [docs/ACKNOWLEDGMENTS.md](docs/ACKNOWLEDGMENTS.md) | 感谢与参考 |
| [docs/](docs/) | 调研与竞品对照 |

## 许可证

[MIT](LICENSE)。Copyright (c) 2026 helixliu。探测核心复刻自 Peter Steinberger 的 CodexBar / SweetCookieKit（同为 MIT），见 [NOTICE](NOTICE)。
