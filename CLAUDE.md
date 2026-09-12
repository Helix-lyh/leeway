# leeway 项目协作指南

`AGENTS.md` 是项目协作主文档，`CLAUDE.md` 同步共有项目指引；仅面向 Codex 的代码生成规则只保留在 `AGENTS.md`，其他共有内容修改时必须同步。

## 工作方法论

维护者本机约定（贡献者不必遵循）：主模型是 Grok 4.6，次模型是 gpt-sol。Grok 负责读现状、改文件、跑验证和收口；需要强推理、方案对照或对抗复核时再交给 gpt-sol。别家模型的接口参数、effort 档位、thinking-block 协议不要写进本仓库规则。

### 第一性原理

- 动手前先回到根本：这个任务到底要解决什么问题？别照搬惯例或“大家都这么做”。
- 把问题拆到最小、能验证的单元，一个个解决。
- 每个决定都说得出“为什么”，而不只是“怎么做”。

### 对抗式审查（交付前必做）

- 写完先切换成最挑剔的审查者，从逻辑漏洞、事实对不对、有没有更简单的做法这几个角度攻击自己。
- 主动列出最可能翻车的 3 到 5 个点，改完再交。
- 不接受“看起来没问题”，得拿出验证过的证据。

## 项目定位

leeway（余量·裕度，模块名 `leeway`）是 AI 编程 provider 用量 / 额度 / 重置时间的纯 CLI 探测工具，复刻 CodexBar 的探测核心。JSON 优先，stdout 只输出数据，方便 agent 直接调用。不做 GUI、菜单栏或后台服务。

支持 provider：**deepseek / kimi / grok / codex / cursor / opencode-go / commandcode**。名单以 README 当前列表为准，不要臆造未实现的家。

入口：`cmd/leeway/main.go`。探测在 `internal/provider/`，CLI 在 `internal/cli/`，JSON 契约在 `internal/model/`。架构见 `docs/DESIGN.md`，各家探测见 `docs/PROVIDER-FETCH.md`。Go 版本以 `go.mod` 为准。

## 命令

```sh
go install ./cmd/leeway
go build -o leeway ./cmd/leeway && ./leeway usage

leeway help
leeway help login

leeway usage
leeway usage cursor work
leeway usage --json
leeway usage --last 24 cursor

leeway status
leeway login cursor
leeway login kimi
leeway logout cursor work
leeway use cursor work
leeway use grok home

go test ./...
```

日常看 `leeway help`；某一条看 `leeway help login`（还有 usage / logout / status / use）。无子命令时往 stderr 打帮助并退出 2。`leeway login cursor/grok/codex/kimi` 走官方 CLI；其余粘贴 key。`leeway use` 把 cursor 写给 Cursor IDE + `~/.cursor/auth.json`（先退出 Cursor；macOS `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`，Linux `~/.config/Cursor/...`），grok 写给 `~/.grok/auth.json`，其余写 DSH；不是切 leeway 自己看哪号。`PROVIDER` / `ACCOUNT` 写在命令后面，也可写成 `--provider` / `--account`。近 N 次点阵用 `usage --last`。

退出码：`0` 全部通过；`1` 至少一个 provider 报错或未配置；`2` 用法 / 配置错误。

## 硬约束

- stdout 只输出数据（JSON 或表格）；日志与错误一律走 stderr。
- 不打印、不复制、不提交密钥、cookie、token、Authorization 或含真实凭据的配置。文档示例只用占位。
- 配置文件 `~/.config/leeway/config.json`（0600，可用 `LEEWAY_CONFIG` 覆盖）；缓存 `~/.cache/leeway/`（`LEEWAY_CACHE`）。凭据优先级：环境变量 → 配置文件。
- 未经明确授权，不访问真实 provider HTTP，也不读用户本机 cookie / keychain 做探测。
- 保持纯 CLI。不要加 GUI、菜单栏、常驻进程或服务模式。
