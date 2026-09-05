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
leeway help                          # 全部命令
leeway usage                         # 已配置的订阅
leeway usage --provider cursor --account work
leeway usage --json                  # 给脚本 / agent

leeway auth status
leeway auth login --provider cursor  # 账号名默认用邮箱；--account work 是 tag
leeway auth login --provider grok --account home
leeway auth login --provider opencode-go --account work
leeway doctor                        # 缺什么、补哪条命令
```

Codex / Kimi 用它们自己的登录：`codex login` / `kimi login`。某一条命令看 `leeway help auth`（还有 usage / cookie / config / use / doctor）。

`--account` 可以是 tag、邮箱（cursor）或订阅名（opencode-go / commandcode）。不写时不读官方 keychain / 共享 `~/.grok`；要看那个号请加 `--account default`。

`leeway use --provider opencode-go --account work` 把 key 写给 DSH，不是切 leeway 自己看哪号。

## 各家怎么登录

| Provider | 登录 |
|---|---|
| cursor | `leeway auth login --provider cursor` |
| grok | `leeway auth login --provider grok` |
| opencode-go / commandcode / deepseek | `leeway auth login --provider …`（粘贴 API key） |
| codex / kimi | `codex login` / `kimi login` |
| grok / kimi 备选 | `leeway cookie import` 或 `--manual`（`leeway help cookie`） |

## 配置

- 配置：`~/.config/leeway/config.json`（0600，`LEEWAY_CONFIG` 可覆盖）
- 缓存：`~/.cache/leeway/`（`LEEWAY_CACHE`）
- 凭据：**环境变量 → 配置文件**。cookie 只在你显式 `import` / `set` 时写入缓存。

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
- `leeway usage --json`：`providers[].accounts[].snapshot.windows[]`（`usedPercent` / `used` / `limit` / `resetsAt`）
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
