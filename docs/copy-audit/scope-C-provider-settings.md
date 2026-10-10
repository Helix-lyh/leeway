# 文案清点 · 范围 C：provider 详情页 + 首次引导页 + 设置页

只读分析，未改动任何代码文件。所有 `file:line` 指向清点时的仓库状态；`原文案` 逐字照抄（含空格与标点），不做任何顺手改写。

行号基准：**清点时的仓库状态**（当时 `internal/gui/*.go` 相对 `HEAD` 还有未提交改动，本报告的行号按当时的工作区算，不是按 `HEAD` 算；这些改动现已随 `906cb5a` 落地，行号会漂移，请以原文案字符串为准检索）。清点时三个文件的校验值：`page_provider.go` 266 行 / 8293 字节、`page_start.go` 91 行 / 2565 字节、`settingsview.go` 333 行 / 11163 字节。

清点范围（按任务划定）：

- `internal/gui/page_provider.go` —— 单个 provider 详情页
- `internal/gui/page_start.go` —— 首次使用引导页
- `internal/gui/settingsview.go` —— 设置页
- 由这三页渲染或触发的、定义在别处的文案：`view.go` 的 `providerHint` / `credSentence` / `loginMethods` / `loginButtonLabel` / `accountActions` / `refreshOptions`，`format.go` 的 `statusText` / `statusLabel` / `amountDetail` / `resetLabel` / `credSentence` / `agoText`，`prompt.go` + `promptview.go` 的登录与确认弹窗，`app.go` 的凭据来源名与操作 toast，`page_matrix.go` 的矩阵列（详情页账号数 > 1 时替换账号卡片），以及详情页每行的走势/预警句子（`trend.go`、`charts.go`、`page_overview.go` 的 `windowMeta`）。

## 条数统计

| 分节 | 条数 |
| --- | --- |
| 表 1 · `page_provider.go`（详情页本体 + 该页每行实际渲染的辅助文案） | 47（本体 28 + 辅助 19） |
| 表 2 · `page_start.go`（首次使用引导页） | 15 |
| 表 3 · `settingsview.go`（设置页） | 50 |
| 表 4 · 由这三页触发、定义在别处（弹窗 + 共享函数） | 93 |
| **合计** | **205** |

同一 `file:line` 出现两条的只有 3 处（`settingsview.go:102`、`:117`、`:269`），因为 `formRow(label, hint)` 把「控件标签」和「说明句」写在同一行，它们是两条独立文案，故拆成两行分别给建议。

## 术语基线（与 `docs/UI-COPY-REWRITE.md` §1 术语对照表对齐）

本报告的建议文案统一使用该术语表的推荐词，避免各范围各改一套：

| 现在有几种叫法 | 建议统一成 |
| --- | --- |
| provider / 家 / 这一家 | **服务商**（行文里可简称「服务」） |
| 凭据 / 登录态 / 令牌 | **登录信息** |
| 探测 / 读取 / 解析一次 | **查询** |
| 会话 / 会话文件 | **登录信息**（状态）/ **登录文件**（文件） |
| 官方客户端 / 官方 CLI / 本机官方客户端 | **官方工具** |
| 切号 | **切换账号** |
| key / API key | **API 密钥** |
| 让本机用它 | **同步到官方工具**（待确认，见「不确定清单」） |
| 私有 HOME | **leeway 自己的登录目录** |
| 点阵 | **走势图** |
| 快照 | **备份一份** |
| cookie | **浏览器登录信息** |

改写规则例外：本范围内有多处是**整句安全说明**（例如「让本机用它」的后果、清除缓存的范围）。这类正文一句话讲不清「会不会动官方工具」，20 字硬上限会削掉最关键的那半句，因此这几行在 `备注` 里明确标注「正文，超 20 字（安全说明）」。所有**控件名称**仍严格遵守 2–4 汉字。

---

## 表 1 · `internal/gui/page_provider.go`（provider 详情页）

| file:line | 原文案 | 位置/控件 | 功能含义 | 建议新文案（名称） | 建议新文案（描述） | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| internal/gui/page_provider.go:31 | `" · 更新于 "` | 页头副标题后缀，拼在 `statusLabel` 后面 | 显示上次探测成功的时间（接 `agoText`，如「更新于 3 分钟前」） | 更新于 | 上一次查到数据的时间 | 拼装字符串，本身不含中文以外的语义 |
| internal/gui/page_provider.go:34 | `"← 总览"` | 页头左侧按钮（`btnLarge(ui.Button)`） | 返回总览页 | 返回总览 | 回到所有服务商 | — |
| internal/gui/page_provider.go:37 | `"刷新"` | 页头主按钮（`ui.PrimaryButton`），`Disabled(a.fetching)` | 立即重新查询该 provider 的用量 | 刷新 | 重新查一遍用量 | 与引导页「刷新看看」、设置页「立刻刷新」不一致，见下表 |
| internal/gui/page_provider.go:40 | `"操作"` | 页头菜单按钮（`ui.MenuButton`） | 打开更多操作菜单 | 更多 | 复制数据、加账号、停用 | 「操作」是空词，扫一眼不知道里面有什么 |
| internal/gui/page_provider.go:41 | `"复制当前 JSON"` | 上述菜单项 | 把本次探测结果 JSON 写入剪贴板（`a.CopyJSON()`） | 复制数据 | 把这次的用量数据复制下来 | JSON 是术语；与设置页同名项重复 |
| internal/gui/page_provider.go:43 | `"已复制 JSON"` | 操作后的 toast | 复制成功反馈 | 已复制数据 | 数据已放进剪贴板 | 与设置页同 toast 重复 |
| internal/gui/page_provider.go:45 | `"添加账号…"` | 上述菜单项 | 打开「添加账号」弹窗，在 config.json 里加一个空账号标签 | 添加账号 | 多加一个账号分别查看 | 省略号表示会开弹窗，保留 |
| internal/gui/page_provider.go:52 | `"打开配置目录"` | 上述菜单项 | 在访达里显示 `config.Path()` 所在位置（只读） | 打开目录 | 在访达里显示配置文件 | 与设置页「打开」按钮同类功能、两种叫法 |
| internal/gui/page_provider.go:61 | `"这一家这次没有答上来"` | 探测失败时的错误行（`errorLine`） | 该 provider 本次探测失败，后接 `staleNote`（如「下面是 3 分钟前的数据」） | 查询失败 | 这个服务商这次没返回数据 | 「答上来」是拟人化口语，与「探测失败」并存的第三套说法 |
| internal/gui/page_provider.go:66 | `"这个 provider 已停用，不会参与探测。"` | 停用状态卡片里的说明行（`unconfiguredLine`） | 该 provider 不参与探测；卡片里配「启用」按钮 | 已停用 | 这个服务商已停用，不再查询用量 | 同一事实在引导页、设置页各有一版不同措辞 |
| internal/gui/page_provider.go:67 | `"启用"` | 停用卡片里的主按钮 | 写回 config.json 打开该 provider，并立即刷新 | 启用 | 重新开始查询这个服务商 | — |
| internal/gui/page_provider.go:86 | `"凭据位置"` | 「凭据位置」卡片的小节标题（`section`） | 卡片标题：说明 leeway 从哪里读登录信息 | 登录信息 | leeway 从哪里读到登录状态 | 「凭据」是安全行业词，见术语基线 |
| internal/gui/page_provider.go:87 | `"会话目录"` | 该卡片 `keyValue` 标签 | 显示 `auth.SessionDir()`：leeway 存放自己那份登录副本的目录 | 存放目录 | leeway 保存登录副本的地方 | 「会话」在中文里先想到聊天 |
| internal/gui/page_provider.go:89 | `"配置文件"` | 该卡片 `keyValue` 标签 | 显示 `config.Path()`（`~/.config/leeway/config.json`） | 配置文件 | leeway 的设置文件，密钥只写这里 | 该文件确实存 API 密钥，描述不能写成「不含密钥」 |
| internal/gui/page_provider.go:92 | `"「让本机用它」只影响本机官方客户端读的那份会话，不改变 leeway 自己看哪个账号。"` | 「凭据位置」卡片里的说明行，**仅当 `len(v.Accounts) > 1` 时渲染** | 解释账号菜单里的高危操作「让本机用它」只改官方工具读的那份登录文件，不改 leeway 自己监控哪个号 | 本机切换 | 会改写官方工具读的登录文件，可能覆盖你现在的登录 | 正文，超 20 字（安全说明）。这是全范围内唯一讲清「让本机用它」作用域的句子，但**单账号时整句不显示**，而单账号正是最常见的情况 |
| internal/gui/page_provider.go:99 | `"停用这个 provider"` | 页头「操作」菜单项（`enabledAction(true)`） | 写回 config.json 停用该 provider | 停用服务 | 不再查询这个服务商，随时可开 | 与设置页开关、引导页说明三种措辞 |
| internal/gui/page_provider.go:101 | `"启用这个 provider"` | 页头「操作」菜单项（`enabledAction(false)`） | 写回 config.json 启用该 provider | 启用服务 | 重新开始查询这个服务商 | 同上 |
| internal/gui/page_provider.go:107 | `"未知"` | `sessionDirText()` 取目录失败时的值 | 会话目录读取失败 | 未知 | 读不到这个目录 | 与 `configPathText`/`cacheDirText`/`historyPathText` 共 4 处同值 |
| internal/gui/page_provider.go:146 | `"这个账号这次没有答上来"` | 账号卡片里的错误行（`errorLine`） | 该账号本次探测失败 | 查询失败 | 这个账号这次没返回数据 | 与第 61 行 provider 级失败只差一个词 |
| internal/gui/page_provider.go:154 | `"凭据可能已失效：重新登录，或让 leeway 再解析一次。"` | 非 `ok`/`not_configured` 账号的兜底说明行 | 提示登录信息可能过期，可重新登录或刷新 | 登录可能已失效 | 重新登录一次，或点刷新再试 | 「再解析一次」是内部黑话，界面上并没有叫「解析」的按钮，用户不知道点哪 |
| internal/gui/page_provider.go:177 | `"（较上次 %+.2f）"` | 费用行里金额后的差值括号 | 本次费用相对上次探测的变化 | 较上次 | 和上一次查询相比的变化 | 与 `deltaText` 的「较上次 %+.0f%%」是两套格式，同一屏可能出现两种 |
| internal/gui/page_provider.go:223 | `"今日"` | 费用行左侧的周期标签（`periodText`，`period == "daily"`） | 费用对应的统计周期 | 今日 | 今天的费用 | — |
| internal/gui/page_provider.go:225 | `"本月"` | 同上（`period == "monthly"`） | 同上 | 本月 | 这个月的费用 | — |
| internal/gui/page_provider.go:227 | `"余额"` | 同上（`period == "balance"`） | 同上 | 余额 | 还剩多少钱 | 与 `amountDetail` 的「余额」同名但含义不同 |
| internal/gui/page_provider.go:229 | `"费用"` | 同上（`period == ""` 的回退值） | 周期未知时的标签 | 费用 | 累计花费 | 与右侧金额语义重复，读起来像「费用 ¥12.00」 |
| internal/gui/page_provider.go:186 | `"provider 没有报告任何额度窗口。"` | 无窗口且无费用时的说明行 | 该 provider 返回了成功但没有额度数据 | 没有额度数据 | 这个服务商没提供用量信息 | 「额度窗口」是 API 词，见术语基线 |
| internal/gui/page_provider.go:196 | `"近 %d 次探测：%s → %s"` | 走势图下的 `historyCaption` 说明行 | 说明走势图覆盖的探测次数与首末时间 | 最近 N 次记录 | 走势图覆盖的记录范围 | 「探测」→「查询/记录」 |
| internal/gui/page_provider.go:257 | `"用量走势："+windowTitle(...)` | 走势图的 `Label`（无障碍名称，不显示） | 给读屏软件的图表名称 | 用量走势 | 这条额度最近的变化 | 与图表下方说明行的「用量走势」表述一致即可 |

### 表 1 附 · 详情页每行（额度卡片）实际渲染、定义在别处的文案

`providerAccount` → `snapshotDetail` → `windowRow` 会调用下列函数，它们的中文会逐字出现在详情页上：

| file:line | 原文案 | 位置/控件 | 功能含义 | 建议新文案（名称） | 建议新文案（描述） | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| internal/gui/format.go:52 | `"已用"` | 额度行标题右侧的次要说明（`amountDetail`） | 百分比窗口的数值含义是已用比例 | 已用 | 已经用掉的比例 | — |
| internal/gui/format.go:54 | `"共 " + money(...)` | 同上，金额窗口 | 已用金额占总额度多少 | 共 | 总额度是多少 | 拼装串，如「共 $20.00」 |
| internal/gui/format.go:56 | `"余额"` | 同上，纯余额窗口 | 数值是剩余余额 | 余额 | 还剩多少钱 | 与 `periodText` 的「余额」同名但含义不同（一个是周期标签，一个是数值说明） |
| internal/gui/format.go:71 | `"等待重置"` | 额度行下的 `resetLabel`（已过期未重置） | 重置时间已过但 provider 未更新 | 等待重置 | 到点了，等官方刷新 | — |
| internal/gui/format.go:73 | `"即将重置"` | 同上（<1 分钟） | 马上重置 | 即将重置 | 一分钟内就恢复 | — |
| internal/gui/format.go:75 | `spanText(left) + "后重置"` | 同上（倒计时） | 如「3 小时 20 分后重置」 | 后重置 | 还有多久恢复额度 | — |
| internal/gui/page_overview.go:735 | `"provider 报告已限流"` | 额度行 `windowMeta` 的末段 | provider 返回限流标记 | 已被限流 | 官方说请求太频繁 | 术语基线建议「请求太频繁」 |
| internal/gui/view.go:492 / 511 | `"较上次 %+.0f%%"` | 额度行 `deltaText` | 与上次探测相比的百分比变化 | 较上次 | 和上一次查询相比的变化 | 与表 1 第 177 行同族 |
| internal/gui/view.go:494 / 515 | `"较上次 %+.2f"` | 额度行 `deltaText`（金额窗口） | 与上次相比的金额变化 | 较上次 | 和上一次查询相比的变化 | 同上 |
| internal/gui/charts.go:118 | `"0–100%"` | 走势图下方 `chartScale` 的刻度说明 | 百分比走势图的纵轴范围 | — | — | 纯数字+符号，不改 |
| internal/gui/charts.go:108 | `"，50% 起告警、80% 起紧张"` | 同上，拼在刻度后 | 说明图上两条色带的阈值 | 50% 提醒 | 用到一半会变色提醒 | 「告警/紧张」是两档情绪词，与状态胶囊的「注意」不是一套 |
| internal/gui/charts.go:328 | `pt.T.Local().Format("01-02 15:04") + " · " + pointText(pt)` | 鼠标悬停/键盘浏览图表时的 `scrubText` | 显示被指到的那一次记录 | — | — | 时间+数值，无中文 |
| internal/gui/trend.go:248 | `"最近 %s每小时涨 %s，约 %s后用满"` | 额度行下方的红色预测句（`burnLine`） | 按最近速率外推，额度会在重置前用满 | 可能提前用满 | 按现在的速度，重置前就会用完 | 这是全窗口唯一允许红字的句子，含义重要；现文案「每小时涨 X%」要用户自己换算 |
| internal/gui/trend.go:251 | `"；重置还有 " + spanText(b.left)` | 同上句尾 | 补充还有多久重置 | 还有 | 距离额度恢复还有多久 | 与 `resetLabel` 的「后重置」是两套说法 |
| internal/gui/trend.go:166 | `"刚重置：%s → %s，额度已恢复"` | 额度行下的 `resetLine`（`resetNote`） | 该额度刚重置，并显示从多少降到多少 | 额度已恢复 | 刚重置，从 X% 降到 Y% | — |
| internal/gui/trend.go:168 | `"刚重置，额度已恢复"` | 同上（拿不到两端数值时） | 只知道重置了 | 额度已恢复 | 额度刚恢复 | — |
| internal/gui/page_matrix.go:149 | `"—"` | 矩阵单元格：该账号没有这个窗口 | 无数值 | — | — | 纯符号 |
| internal/gui/page_matrix.go:153 | `"未报告"` | 矩阵单元格：有窗口但无用量字段 | 该窗口没数据 | 未报告 | 这个周期官方没给数据 | — |
| internal/gui/trend.go:134 | `"约 %s后用满"` | 矩阵列里的预警（`burnAlarmInColumn`） | 列宽不够，只留结论 | 约 X 后用满 | 按现速多久用满 | 与 `burnLine` 的长句并存，同一事实两种长度 |

---

## 表 2 · `internal/gui/page_start.go`（首次使用引导页）

触发条件：总览页在「还没探到任何一家」时改画这张引导卡（`page_overview.go:73`、`:106`）。

| file:line | 原文案 | 位置/控件 | 功能含义 | 建议新文案（名称） | 建议新文案（描述） | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| internal/gui/page_start.go:16 | `"开始使用"` | 引导卡的小节标题（`section`） | 卡片标题 | 开始使用 | 先配好一个就能看用量 | — |
| internal/gui/page_start.go:17 | `"leeway 不替你登录，也不碰你的密码：它读你已有的凭据，问 provider 自己还剩多少额度。"` | 卡片首段说明（`ui.Text`，MaxWidth 620） | 说明 leeway 不采集密码，只读本机已有登录信息去查用量 | 不改你的登录 | leeway 不碰你官方工具的登录，只读取已有的登录信息去查用量 | 正文，超 20 字（安全说明）。「不替你登录」与下方每行的「用官方 CLI 登录」按钮字面冲突，用户会问「那你刚才点的是什么」 |
| internal/gui/page_start.go:28 | `"配好一个就能用：点「刷新」或按 ⌘R。命令行照旧：leeway usage"` | 卡片末尾的 `muted` 说明行 | 告诉用户配好后怎么刷新；顺带提示 CLI 仍可用 | 配好一个就能用 | 点「刷新」或按 ⌘R 看结果 | 在图形界面首屏塞 CLI 命令，对首次使用者是噪音；「命令行照旧」语义不明（照旧什么） |
| internal/gui/page_start.go:46 | `"已停用，不参与探测。"` | provider 行名称下的 `muted` 说明（`!v.Enabled` 分支） | 该 provider 被停用 | 已停用 | 不再查询这个服务商 | 与表 1:66、表 3:261 同义不同文 |
| internal/gui/page_start.go:59 | `"已找到凭据"` | 名称右侧的绿色胶囊（`pill`，`toneOK`） | 本机已存在该 provider 的登录信息，还没查过 | 已登录 | 找到登录信息，还没查 | 与状态胶囊的「待刷新/未配置」不是一套词；「凭据」见术语基线 |
| internal/gui/page_start.go:67 | `"启用"` | 停用行的按钮 | 写回 config.json 启用 | 启用 | 重新开始查询这个服务商 | 与表 1:67 一致 |
| internal/gui/page_start.go:71 | `"刷新看看"` | 已有登录信息行的按钮 | 触发全局 `Refresh(true)` | 刷新 | 现在查一次用量 | 「看看」是口语尾缀；与「刷新」「立刻刷新」三种叫法 |
| internal/gui/page_start.go:80 | `"详情"` | 每行右侧按钮 | 跳到该 provider 详情页 | 详情 | 看这个服务商的完整数据 | 与表 3:248 一致 |
| internal/gui/page_start.go:90 | `startAction` → `"粘贴 API key"` 或 `"用官方 CLI 登录"` | 每行主按钮标签，取 `loginMethods(id)[0].label` 去掉省略号 | deepseek/opencode-go/commandcode 显示「粘贴 API key」；cursor/grok/codex/kimi 显示「用官方 CLI 登录」（已按 AuthSpec 核对） | 填 API 密钥 / 官方登录 | 填密钥后才能查 / 交给官方工具登录 | 同一列的按钮一半是「粘贴…」（动作）一半是「用…登录」（方式），句式不齐；「key」「CLI」都是术语 |
| internal/gui/view.go:663 | `"还没有 API key。"` | `providerHint`，deepseek / opencode-go / commandcode | 该 provider 没有登录信息 | 未登录 | 还没有 API 密钥 | 见「表 4」共享函数，此处只列引导页会用到的那几条 |
| internal/gui/view.go:665 | `"还没有 cursor 私有会话。先用官方 cursor-agent 登录。"` | `providerHint`，cursor | 没有 cursor 登录信息 | 未登录 | 先用官方 cursor-agent 登录 | 「私有会话」是内部说法 |
| internal/gui/view.go:667 | `"还没有 grok 会话。用官方 grok 登录，或导入 ~/.grok/auth.json。"` | `providerHint`，grok | 同上 | 未登录 | 用官方 grok 登录，或导入登录文件 | 「会话」→「登录文件」 |
| internal/gui/view.go:669 | `"还没有 codex 会话。用官方 codex 登录，或导入 ~/.codex/auth.json。"` | `providerHint`，codex | 同上 | 未登录 | 用官方 codex 登录，或导入登录文件 | 同上 |
| internal/gui/view.go:671 | `"还没有 kimi 凭据。官方 kimi 登录、API key 或浏览器 cookie 都可以。"` | `providerHint`，kimi | 同上 | 未登录 | 官方登录、API 密钥或浏览器登录都行 | 一句话塞三种方式，且三种术语并列 |
| internal/gui/view.go:673 | `"还没有凭据。"` | `providerHint` 兜底 | 未知 provider 没有登录信息 | 未登录 | 还没有登录信息 | — |

---

## 表 3 · `internal/gui/settingsview.go`（设置页）

| file:line | 原文案 | 位置/控件 | 功能含义 | 建议新文案（名称） | 建议新文案（描述） | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| internal/gui/settingsview.go:41 | `"窗口偏好写在 gui.json，与命令行的 config.json 分开"` | 设置窗口标题栏里的一行说明（`muted` + Tooltip） | 说明界面偏好存 `~/.config/leeway/gui.json`，与 CLI 的 config.json 分开 | 界面设置 | 存在 leeway 自己的文件里，不影响命令行 | 「窗口偏好」「gui.json」都是内部名词；用户不知道这两个文件是什么 |
| internal/gui/settingsview.go:71 | `"设置"` | 设置窗口标题栏（`chrome`） | 窗口名 | 设置 | — | — |
| internal/gui/settingsview.go:81 | `"通用"` / `"Provider"` / `"数据"` | 标题栏右侧 `ui.Tabs` 三个标签 | 三个设置分页 | 通用 / 服务商 / 数据 | — | 只有中间一个用英文，中文界面里最扎眼 |
| internal/gui/settingsview.go:101 | `"刷新"` | 「刷新」卡片小节标题 | 刷新节奏相关设置 | 刷新 | — | — |
| internal/gui/settingsview.go:102 | `"自动刷新"` | `formRow` 标签（下拉框上方） | 自动刷新间隔下拉 | 自动刷新 | 多久自动查一次 | — |
| internal/gui/settingsview.go:102 | `"自动刷新读一分钟内的缓存；手动刷新总是重新探测。"` | 该行的 `formRow` 说明 | 自动刷新复用 1 分钟内的结果，手动刷新强制重查 | 自动刷新 | 一分钟内直接用上次结果，手动刷新会重查 | 「读缓存」「探测」是术语；未确认这 1 分钟缓存属于 leeway 还是 provider，见不确定清单 |
| internal/gui/view.go:307 | `"手动刷新"` | 自动刷新下拉的第一个选项 | 关闭自动刷新 | 不自动刷新 | 只在我点刷新时查 | 同一列表里前一项是「模式」，后四项是「间隔」，读起来不同类 |
| internal/gui/view.go:308 | `"每 30 秒"` | 下拉选项 | 30 秒间隔 | 每 30 秒 | — | — |
| internal/gui/view.go:309 | `"每分钟"` | 下拉选项 | 60 秒间隔 | 每分钟 | — | 与「每 30 秒/每 5 分钟」句式不齐（少了「每…」里的数字） |
| internal/gui/view.go:310 | `"每 5 分钟"` | 下拉选项 | 300 秒间隔 | 每 5 分钟 | — | — |
| internal/gui/view.go:311 | `"每 15 分钟"` | 下拉选项 | 900 秒间隔 | 每 15 分钟 | — | — |
| internal/gui/settingsview.go:117 | `"探测超时（秒）"` | `formRow` 标签（数字输入框上方） | 单个 provider 的 HTTP 超时秒数 | 超时（秒） | 单个服务商最多等多久 | 「探测」→「查询」 |
| internal/gui/settingsview.go:117 | `"单个 provider 的 HTTP 超时，等价于 LEEWAY_TIMEOUT。"` | 该行说明 | 说明该值等于 CLI 环境变量 `LEEWAY_TIMEOUT` | 超时（秒） | 单个服务商最多等多久，等于 LEEWAY_TIMEOUT | 对图形界面用户，环境变量名是噪音；对 CLI 用户有用，可折进 tooltip |
| internal/gui/settingsview.go:128 | `"外观"` | 卡片小节标题 | 主题与窗口行为 | 外观 | — | — |
| internal/gui/settingsview.go:129 | `"主题"` | `formRow` 标签（主题选择器上方） | 主题选择 | 主题 | — | — |
| internal/gui/settingsview.go:130 | `"打开窗口时探测"` | `toggleRow` 开关名 | 开窗即触发一次探测 | 启动刷新 | 打开窗口就查一次用量 | 「探测」→「查询/刷新」 |
| internal/gui/settingsview.go:131 | `"探测失败时发通知"` | `toggleRow` 开关名 | 探测失败且窗口不在前台时发系统通知 | 失败提醒 | 查询失败时发系统通知 | — |
| internal/gui/settingsview.go:133 | `"这个环境不支持系统通知（macOS 只在打包安装后的构建里允许）。"` | 通知开关下的条件说明行 | 当前运行环境不支持通知 | 不支持通知 | 要装打包好的版本才能用 | 「打包安装后的构建」是开发说法 |
| internal/gui/settingsview.go:138 | `"启动"` | 卡片小节标题 | 开机自启设置 | 启动 | — | — |
| internal/gui/settingsview.go:139 | `"开机自动启动"` | `toggleRow` 开关名 | 写 macOS 登录项（`mygo.App.SetOpenAtLogin`） | 开机启动 | 开机后自动打开 leeway | 会改系统登录项，不是只改 leeway 自己的配置 |
| internal/gui/settingsview.go:147 | `"需要打包后的 .app（macOS 13+）；未打包运行时这个开关会失败并提示原因。"` | 开关下的说明行 | 说明该开关只在打包版本可用 | 需要打包版本 | 要安装 .app 才能用（macOS 13 以上） | 已说清失败原因，较好 |
| internal/gui/settingsview.go:160 | `"跟随系统"` | 主题选择器第一个色块的标签 | 跟随系统深浅色 | 跟随系统 | 跟系统深浅色走 | — |
| internal/gui/settingsview.go:161 | `"浅色"` | 第二个色块 | 浅色主题 | 浅色 | — | — |
| internal/gui/settingsview.go:162 | `"深色"` | 第三个色块 | 深色主题 | 深色 | — | — |
| internal/gui/settingsview.go:172 | `"主题："+opt.label` | 主题色块的 `Label`（无障碍名称） | 读屏名称，如「主题：浅色」 | 主题 | — | 与可见标签重复，但无障碍需要 |
| internal/gui/settingsview.go:238 | `"启用的 provider"` | 卡片小节标题 | provider 启用开关列表 | 服务商开关 | 决定哪些服务商参与查询 | 「启用的 provider」是定语+英文名词，读起来别扭 |
| internal/gui/settingsview.go:248 | `"详情"` | 每行按钮 | 跳到该 provider 详情页并前置主窗口 | 详情 | 看这个服务商的完整数据 | 与引导页一致 |
| internal/gui/settingsview.go:256 | `"启用 " + view.DisplayName` | 开关的 `Label`（无障碍名称） | 读屏名称，如「启用 cursor」 | 启用 | — | 与可见行名重复，无障碍需要 |
| internal/gui/settingsview.go:261 | `"停用的 provider 不参与探测。登录、切号和退出在各自的详情页里。"` | 列表底部说明行 | 说明停用后果，并指路详情页 | 已停用 | 停用的服务商不再查询用量；登录和切换账号在详情页 | 「切号」是圈内缩写；「退出」指退出登录，与「切号」并列容易误读为退出 leeway |
| internal/gui/settingsview.go:268 | `"历史"` | 卡片小节标题 | 历史记录设置 | 历史 | — | — |
| internal/gui/settingsview.go:269 | `"每个窗口保留的探测次数"` | `formRow` 标签（数字输入框上方） | 每个额度周期保留多少条历史 | 保留条数 | 每项额度最多记多少条 | 「窗口」是 API 词；「探测次数」→「记录条数」 |
| internal/gui/settingsview.go:269 | `"影响历史页的点阵，也是 CLI 里 usage --last 的默认次数。"` | 该行说明 | 影响历史页走势图点数与 `usage --last` 默认值 | 保留条数 | 影响历史走势图，也是 usage --last 的默认值 | 「点阵」是打印机名词 |
| internal/gui/settingsview.go:281 | `"文件"` | 卡片小节标题 | leeway 用的四个文件/目录 | 文件 | — | — |
| internal/gui/settingsview.go:283 | `"配置"` | 文件行标签 | 显示 config.json 路径 | 配置 | — | — |
| internal/gui/settingsview.go:284 | `"缓存"` | 文件行标签 | 显示 `auth.CacheDir()` | 缓存 | — | — |
| internal/gui/settingsview.go:285 | `"会话"` | 文件行标签 | 显示 `auth.SessionDir()` | 登录 | leeway 保存登录副本的地方 | 详情页叫「会话目录」、弹窗叫「会话文件」，这里只有一个「会话」 |
| internal/gui/settingsview.go:286 | `"历史"` | 文件行标签 | 显示 `history.Path()` | 历史 | — | 与卡片标题「历史」重名，同屏出现两个「历史」 |
| internal/gui/settingsview.go:299 | `file.path + "\n点击复制路径"` | 路径文本的 Tooltip | 提示点击可复制路径 | 点击复制路径 | — | — |
| internal/gui/settingsview.go:302 | `"已复制路径"` | 复制后的 toast | 复制成功反馈 | 已复制路径 | 路径已放进剪贴板 | 与「已复制数据」句式不一致（一个带宾语一个不带） |
| internal/gui/settingsview.go:304 | `"打开"` | 文件行按钮 | 在访达里显示该文件（`ShowItemInFolder`） | 打开位置 | 在访达里显示这个文件 | 「打开」看不出打开什么；详情页同功能叫「打开配置目录」 |
| internal/gui/settingsview.go:312 | `"维护"` | 卡片小节标题 | 数据维护按钮组 | 维护 | — | — |
| internal/gui/settingsview.go:314 | `"复制当前 JSON"` | 按钮 | 复制本次探测 JSON | 复制数据 | 把这次的用量数据复制下来 | 与详情页同名项重复 |
| internal/gui/settingsview.go:316 | `"已复制 JSON"` | toast | 复制成功反馈 | 已复制数据 | 数据已放进剪贴板 | 与详情页同 toast 重复 |
| internal/gui/settingsview.go:318 | `"清除 cookie 缓存"` | 按钮，点后弹确认框 | 删除 leeway 缓存的浏览器 cookie 导入结果 | 清除缓存 | 下次查询重新读浏览器登录 | 「cookie」是术语 |
| internal/gui/settingsview.go:321 | `"立刻刷新"` | 按钮 | 立即重新探测全部 provider | 刷新 | 现在重新查一遍 | 与「刷新」「刷新看看」三种叫法 |
| internal/gui/settingsview.go:325 | `"清除缓存只让下一次探测重新读一遍浏览器 cookie，不动 config.json 里的密钥。"` | 「维护」卡片底部说明行 | 说明清缓存的边界：只影响缓存，不删密钥 | 清除缓存 | 只让下次查询重读浏览器登录，不删密钥，也不动官方工具 | 正文，超 20 字（安全说明）。它回答了「会不会删我的密钥」，但没回答用户真正怕的「会不会动官方工具」 |
| internal/gui/settingsview.go:329 | `"关于"` | 卡片小节标题 | 版本信息 | 关于 | — | 与侧栏「关于」页同名，两处内容重复（见总结） |
| internal/gui/settingsview.go:330 | `"版本"` | `keyValue` 标签 | 显示 `leeway <version>` | 版本 | — | — |
| internal/gui/settingsview.go:331 | `"界面"` | `keyValue` 标签 | 显示 `go <goVersion()>`，即 Go 运行时版本 | 运行环境 | leeway 编译用的 Go 版本 | 「界面 go1.24」读起来像界面自己的版本号，实际是运行时 |
| internal/gui/settingsview.go:143 | `"开机自启设置失败"` | `failureText` 的错误句 → toast | 写登录项失败 | 开机启动设置失败 | 没能改成开机启动 | 与开关名「开机自动启动」不一致 |

---

## 表 4 · 由这三页触发、定义在别处的文案（弹窗与共享函数）

这些字符串不在三个文件里，但用户是从详情页/引导页/设置页点出来的，属于同一段体验。

| file:line | 原文案 | 位置/控件 | 功能含义 | 建议新文案（名称） | 建议新文案（描述） | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| internal/gui/view.go:526 | `"账号操作"` | 账号卡片右侧菜单按钮 | 打开该账号的操作菜单 | 账号 | 登录、切换、退出都在这 | 与矩阵列同名（page_matrix.go:118） |
| internal/gui/view.go:530 | `"让本机用它"` | 账号菜单项，**最高危操作** | 把该账号的登录写入官方工具（cursor/grok/kimi；codex 不出现） | 同步到官方工具 | 会让官方工具改用这个账号 | 待确认，见不确定清单。现名完全看不出会覆盖官方工具的登录 |
| internal/gui/view.go:535 | `"复制当前 JSON"` | 账号菜单项 | 复制 JSON | 复制数据 | 把这次的用量数据复制下来 | 与表 1/表 3 重复 |
| internal/gui/view.go:548 | `"退出登录"` | 账号菜单项（有会话的账号） | 删除 leeway 保存的私有登录（会话目录 + 配置），**不动官方工具** | 退出登录 | 只退出 leeway 里的这个账号，不动官方工具 | 标签本身像「退出官方工具」，恐惧感主要来自这里 |
| internal/gui/view.go:552 | `"删除 API key"` | 账号菜单项（只有 key 的账号） | 只删 leeway 里存的 key | 删除密钥 | 只删 leeway 存的 API 密钥 | 与「退出登录」并列时，用户会以为两个都会动官方工具 |
| internal/gui/view.go:575 | `"粘贴 API key…"` | 登录菜单项 | 打开粘贴密钥弹窗 | 填 API 密钥 | 粘贴后只存在 leeway 里 | — |
| internal/gui/view.go:576 | `"从浏览器导入 cookie…"` | 登录菜单项 | 打开浏览器导入弹窗 | 从浏览器导入 | 读浏览器的登录状态 | — |
| internal/gui/view.go:577 | `"粘贴 cookie header…"` | 登录菜单项 | 打开手动粘贴 cookie 弹窗 | 手动粘贴登录 | 从开发者工具复制过来 | — |
| internal/gui/view.go:578 | `"导入会话文件…"` | 登录菜单项 | 打开导入会话文件弹窗 | 导入登录文件 | 用官方工具已经生成的登录文件 | — |
| internal/gui/view.go:579 | `"用官方 CLI 登录…"` | 登录菜单项，cursor/grok/codex/kimi | 用私有 HOME 运行官方 CLI 登录，**不动官方工具** | 官方登录 | 交给官方工具登录，不动你现有的登录 | 「CLI」是术语；「不动官方工具」是这句最该带上的信息 |
| internal/gui/view.go:612 | `"登录"` | 账号卡片登录按钮（有官方 CLI 的 provider） | 打开登录菜单 | 登录 | — | — |
| internal/gui/view.go:614 | `"配置 key"` | 账号卡片登录按钮（只有 key 的 provider） | 打开登录菜单 | 配置密钥 | 填 API 密钥才能查用量 | 与引导页按钮「粘贴 API key」不是一套词 |
| internal/gui/prompt.go:267 | `"让本机 %s 改用 %s？"` | 「让本机用它」确认框标题 | 确认把官方工具切到该账号 | 让官方工具改用这个账号？ | — | 「本机 %s」中 %s 是英文 provider id |
| internal/gui/prompt.go:268 | `"leeway 会把该账号的会话写给本机官方客户端；本机当前登录会被快照成 leeway 账号。"` | 确认框正文 | 说明会写官方配置，并先备份当前登录 | 会改写官方工具的登录文件 | 你现在的登录会先备份进 leeway，可以再切回来 | 正文，超 20 字（安全说明）。「快照」是运维词；「本机官方客户端」与「官方工具」应统一；未说明会覆盖哪些具体位置 |
| internal/gui/prompt.go:269 | `"切换"` | 确认框危险按钮 | 执行 use | 切换 | 立刻改写官方工具的登录 | 按钮只有两个字，没承担警示 |
| internal/gui/prompt.go:280 | `"退出 %s 账号 %s？"` | 「退出登录」确认框标题 | 确认删除 leeway 里的该账号登录 | 退出这个账号？ | — | — |
| internal/gui/prompt.go:281 | `"会删除该账号在 leeway 里保存的私有会话、cookie 与配置里的 key，不影响本机官方客户端里的登录（cursor/grok 还会清掉私有 HOME）。"` | 确认框正文 | 说明只删 leeway 自己的副本，不动官方工具 | 只删 leeway 里的登录 | 官方工具里的登录不动，可以随时再登录回来 | 正文，超 20 字（安全说明）。句子太长且把最关键的「不影响官方工具」放在后半句；「私有 HOME」「私有会话」是内部说法 |
| internal/gui/prompt.go:283 | `"删除 %s 账号 %s 的 API key？"` | 无会话账号的确认框标题 | 确认删除 leeway 存的 key | 删除这个 API 密钥？ | — | — |
| internal/gui/prompt.go:284 | `"只删掉 leeway 里存的这个 key，本机其它地方不受影响。"` | 确认框正文 | 只删 leeway 的 key | 只删 leeway 存的密钥 | 官方工具和其它地方都不受影响 | 这句是全范围内说得最清楚的一条 |
| internal/gui/prompt.go:290 | `removeAccountLabel(session)` → `"退出登录"` / `"删除 API key"` | 确认框危险按钮 | 同上，按钮复用菜单名 | 退出登录 / 删除密钥 | — | 危险按钮与菜单项同名，没有加重语气 |
| internal/gui/prompt.go:313 | `"清除 cookie 缓存？"` | 清除缓存确认框标题 | 确认删除缓存文件 | 清除缓存？ | — | — |
| internal/gui/prompt.go:314 | `"下一次探测会重新从浏览器读一次 cookie。config.json 里的密钥和账号不受影响。"` | 确认框正文 | 说明清缓存的范围 | 下次查询会重新读浏览器登录 | 密钥和账号不受影响，官方工具也不受影响 | 正文，超 20 字（安全说明） |
| internal/gui/prompt.go:315 | `"清除"` | 确认框危险按钮 | 执行清除 | 清除 | — | — |
| internal/gui/promptview.go:85 | `"粘贴 API key"` | 弹窗标题（`promptHeader`） | API key 弹窗 | 填 API 密钥 | — | 与按钮「配置 key」、菜单「粘贴 API key…」三种叫法 |
| internal/gui/promptview.go:87 | `"从浏览器导入 cookie"` | 弹窗标题 | 浏览器导入弹窗 | 从浏览器导入 | — | — |
| internal/gui/promptview.go:89 | `"粘贴 cookie header"` | 弹窗标题 | 手动 cookie 弹窗 | 手动粘贴登录 | — | — |
| internal/gui/promptview.go:91 | `"导入会话文件"` | 弹窗标题 | 会话文件弹窗 | 导入登录文件 | — | — |
| internal/gui/promptview.go:93 | `"添加账号"` | 弹窗标题 | 新账号弹窗 | 添加账号 | — | — |
| internal/gui/promptview.go:95 | `"用官方 CLI 登录"` | 弹窗标题 | 官方 CLI 登录弹窗 | 官方登录 | — | — |
| internal/gui/promptview.go:97 | `"登录"` | `promptHeader` 兜底 | 未知类型弹窗 | 登录 | — | — |
| internal/gui/promptview.go:107 | `" · 账号 "` + account | 弹窗副标题 | 标明弹窗作用于哪个账号 | 账号 | — | — |
| internal/gui/promptview.go:114 | `"停止"` | CLI 登录弹窗取消按钮（运行中） | 取消正在跑的官方 CLI | 停止 | 中断这次官方登录 | — |
| internal/gui/promptview.go:117 | `"关闭"` | CLI 登录弹窗取消按钮（结束后） | 关闭弹窗 | 关闭 | — | — |
| internal/gui/promptview.go:119 | `"取消"` | 其它弹窗取消按钮 | 关闭弹窗 | 取消 | — | — |
| internal/gui/promptview.go:65 | `"保存"` | 弹窗主按钮（非 CLI） | 提交表单 | 保存 | — | — |
| internal/gui/promptview.go:67 | `"完成"` | CLI 弹窗主按钮（结束后） | 关闭并完成 | 完成 | — | — |
| internal/gui/promptview.go:136 | `"key 会写进 ~/.config/leeway/config.json（0600），不打印、不上传。"` | API key 输入框下的说明 | 说明密钥只写本地文件 | 只存在这台电脑 | 写进 leeway 自己的配置文件，不上传 | 「0600」是文件权限术语 |
| internal/gui/promptview.go:138 | `"已经有一把 key：留空就是保持不变，填新的会覆盖。"` | 已有密钥时的说明 | 留空保留原密钥 | 已有密钥 | 留空就不改，填新的会覆盖 | 「一把 key」量词随意 |
| internal/gui/promptview.go:150 | `"显示明文"` | 复选框 | 切换密钥明文显示 | 显示明文 | — | — |
| internal/gui/promptview.go:156 | `"浏览器"` | `formRow` 标签 | 浏览器名输入框 | 浏览器 | 用哪个浏览器登录的 | — |
| internal/gui/promptview.go:159 | `"Profile（可留空）"` | `formRow` 标签 | 浏览器 profile 输入框 | 浏览器配置 | 不知道就留空 | 英文词+括号，中文界面里突兀 |
| internal/gui/promptview.go:160 | `"leeway 从浏览器的 cookie 库里读出这个 provider 的登录态，只存在你自己的缓存目录。"` | 该行说明 | 说明读取范围与存放位置 | 只读浏览器的登录状态 | 读到的内容只存在 leeway 自己的缓存里 | 「cookie 库」「登录态」是术语 |
| internal/gui/promptview.go:166 | `"cookie header"` | `formRow` 标签 | 手动 cookie 输入框 | 登录信息 | 从开发者工具复制 Cookie 请求头 | 标签直接用英文 |
| internal/gui/promptview.go:167 | `"适合浏览器导入不成功的场景：从开发者工具里复制 Cookie 请求头。这个框是明文，旁边有人或录屏时注意。"` | 该行说明 | 说明用途与明文风险 | 手动粘贴登录 | 浏览器导入失败时用；这里是明文，注意旁人 | 说清了明文风险，较好；「场景」偏书面 |
| internal/gui/promptview.go:170 | `"name=value; name2=value2（curl -b 或 Cookie: 片段也行）"` | 输入框 Placeholder | 输入格式示例 | — | — | 格式示例，技术性可接受 |
| internal/gui/promptview.go:175 | `"会话文件"` | `formRow` 标签 | 会话文件路径输入框 | 登录文件 | 官方工具生成的登录文件 | — |
| internal/gui/promptview.go:176 | `"leeway 把这份文件快照到自己的会话目录，之后切号不会动原文件。"` | 该行说明 | 说明是复制一份，不动原文件 | 只复制一份 | 复制进 leeway 自己的目录，原文件不动 | 「快照」「切号」是内部说法；这句是导入路径的关键安全承诺，写得很好但用词太内部 |
| internal/gui/promptview.go:179 | `"选择…"` | 文件选择按钮 | 打开系统文件选择器 | 选择文件 | 从访达里挑一个文件 | — |
| internal/gui/promptview.go:186 | `"账号名"` | `formRow` 标签 | 新账号名输入框 | 账号名 | 只是标签，随便起 | — |
| internal/gui/promptview.go:187 | `"只是个标签，之后可以给它单独登录一份凭据，再用「让本机用它」切换。"` | 该行说明 | 说明账号只是个标签 | 只是个名字 | 之后可以单独登录，再切换官方工具 | 引用了最高危操作的名字却没说它会写官方配置 |
| internal/gui/promptview.go:203 | `"等待官方 CLI 完成登录…"` | CLI 弹窗状态行 | 登录进行中 | 等待官方工具登录… | 请在官方工具窗口里完成登录 | — |
| internal/gui/promptview.go:205 | `"登录完成"` | CLI 弹窗状态行 | 登录成功 | 登录完成 | — | — |
| internal/gui/promptview.go:214 | `"（官方 CLI 还没有输出）"` | 日志区占位 | 官方 CLI 暂无输出 | 官方工具还没有输出 | — | — |
| internal/gui/promptview.go:229 | `"登录在 leeway 自己的 HOME 里进行，不会覆盖你本机官方客户端的登录。"` | CLI 弹窗底部说明行 | 说明官方登录跑在私有 HOME，不动官方工具 | 不会动你现有的登录 | 登录在 leeway 自己的目录里进行 | 这是全范围内最关键的一句安全承诺，但「HOME」是 shell 变量名，普通用户读不懂 |
| internal/gui/promptview.go:238 | `"选择会话文件"` | 系统文件选择对话框标题 | 选文件对话框 | 选择登录文件 | — | — |
| internal/gui/app.go:556 | `"账号 API key"` | `credentialInfo` 返回的凭据来源名，显示在详情页凭据行与 `credSentence` 里 | 该账号的密钥来自账号级配置 | 账号密钥 | — | 这些来源名会拼进「凭据来自 X」 |
| internal/gui/app.go:558 | `"环境变量 " + spec.EnvAPIKey[0]` | 同上 | 来自环境变量 | 环境变量 | — | 如「环境变量 DEEPSEEK_API_KEY」 |
| internal/gui/app.go:560 | `"环境变量 " + spec.EnvAuthToken[0]` | 同上 | 来自 token 环境变量 | 环境变量 | — | — |
| internal/gui/app.go:562 | `"配置里的 API key"` | 同上 | 来自 provider 级配置 | 配置里的密钥 | — | — |
| internal/gui/app.go:564 | `"会话文件 " + shortenPath(acc.AuthFile)` | 同上 | 来自账号绑定的会话文件 | 登录文件 | — | — |
| internal/gui/app.go:566 | `"手动 cookie"` | 同上 | 来自手动粘贴的 cookie | 手动粘贴 | — | — |
| internal/gui/app.go:570 | `"cookie"` | 同上 | 来自浏览器 cookie 缓存 | 浏览器登录 | — | — |
| internal/gui/app.go:575 | `"Codex CLI 会话"` | 同上 | 复用 `~/.codex/auth.json` | Codex 登录 | — | — |
| internal/gui/app.go:586 | `"cursor 私有会话"` | 同上 | cursor 的私有登录 | cursor 登录 | — | — |
| internal/gui/app.go:588 | `" 过期"` | 同上，拼在到期时间后 | 令牌到期时间 | 过期 | — | 如「01-02 15:04 过期」，与详情页 `av.Expires` 拼接 |
| internal/gui/format.go:87 | `"凭据来自" + gap + cred + "；刷新一次就能看到用量。"` | `credSentence`，详情页账号卡片与引导页每行的说明 | 说明登录信息来源，并提示刷新 | 登录来自 X | 刷新一次就能看到用量 | 「凭据」→「登录」；与「已找到凭据」并列出现时是两种称呼 |
| internal/gui/format.go:168 | `"正常"` | `statusText`，状态胶囊 | 探测成功 | 正常 | — | — |
| internal/gui/format.go:170 | `"失败"` | 同上 | 探测失败 | 失败 | — | 与错误行「查询失败」不同词 |
| internal/gui/format.go:172 | `"未配置"` | 同上 | 没有登录信息 | 未配置 | — | 与引导页胶囊「已找到凭据」不成对 |
| internal/gui/format.go:174 | `"已停用"` | 同上 | 被停用 | 已停用 | — | — |
| internal/gui/format.go:176 | `"注意"` | 同上 | 告警状态 | 注意 | — | 与图表说明的「起告警/起紧张」不是一套 |
| internal/gui/format.go:186 | `"待刷新"` | `statusLabel`，有登录信息但未探测 | 已找到登录信息，尚未查询 | 待刷新 | 找到登录信息，还没查 | 与引导页「已找到凭据」同义不同文 |
| internal/gui/format.go:146 | `"尚未探测"` | `agoText` 的零值 | 从未成功查询 | 还没查过 | — | 「探测」→「查询」 |
| internal/gui/format.go:151 | `"刚刚"` | `agoText` | 5 秒内 | 刚刚 | — | — |
| internal/gui/format.go:153 | `"%d 秒前"` | `agoText` | 相对时间 | — | — | — |
| internal/gui/format.go:155 | `"%d 分钟前"` | `agoText` | 相对时间 | — | — | — |
| internal/gui/format.go:157 | `"%d 小时前"` | `agoText` | 相对时间 | — | — | — |
| internal/gui/format.go:159 | `"%d 天前"` | `agoText` | 相对时间 | — | — | — |
| internal/gui/view.go:398 | `"下面是 " + agoText(...) + "的数据"` | `staleNote`，详情页错误行拼接 | 说明数据是旧的 | 这是上次的数据 | 上次查到的时间 | — |
| internal/gui/app.go:659 | `"已切换 " + provider + " 到 " + account` | `Use` 成功 toast | 切换成功 | 已切换 | 官方工具现在用这个账号 | — |
| internal/gui/app.go:661 | `"（原账号已存为 " + res.SavedAs + "）"` | 同上 | 原登录已备份 | 原账号已备份 | 原来的登录存成了 leeway 账号 | — |
| internal/gui/app.go:664 | `"；原登录不在 leeway 台账里，切走之后只剩目标号"` | 同上 | 无法备份原登录的警告 | 原登录没备份 | 切走后原来的登录就找不回来了 | 「台账」是内部词，且这句是真正的不可逆警告，被塞在 toast 括号里 |
| internal/gui/app.go:667 | `"（写入 " + strings.Join(res.Targets, "、") + "）"` | 同上 | 列出写入的官方位置 | 写入 | 改写了哪些文件 | 位置列表本身是技术信息，但对「动了哪里」很有用 |
| internal/gui/app.go:656 | `"切换失败"` | `failureText` 错误句 | use 失败 | 切换失败 | — | — |
| internal/gui/app.go:682 | `"退出失败"` | `failureText` 错误句 | logout 失败 | 退出失败 | — | — |
| internal/gui/app.go:696 | `"设置保存失败"` | `failureText` 错误句 | 保存 gui.json 失败 | 设置保存失败 | — | — |
| internal/gui/app.go:711 | `"清除缓存失败"` | `failureText` 错误句 | 清缓存失败 | 清除缓存失败 | — | — |
| internal/gui/app.go:714 | `"已清除 cookie 缓存"` | 清缓存成功 toast | 清除成功 | 已清除缓存 | 下次查询会重新读浏览器登录 | — |
| internal/gui/app.go:724 | `"保存失败"` | `failureText` 错误句 | 写 config.json 失败 | 保存失败 | — | 与「设置保存失败」并存，用户分不清哪个没保存 |
| internal/gui/app.go:306 | `"探测失败"` | `failureText` 错误句，屏幕阅读器与系统通知 | 探测失败 | 查询失败 | — | — |
| internal/gui/app.go:316 | `"刷新完成：" + a.summary()` | 刷新完成播报 | 读屏播报结果 | 刷新完成 | — | — |
| internal/gui/app.go:327 | `"%d 正常 · %d 失败 · %d 未配置"` | `summary`，标题栏与读屏 | 结果统计 | — | — | 与状态胶囊同词，较好 |
| internal/gui/app.go:341 | `"leeway 探测失败"` | 系统通知标题 | 通知标题 | leeway 查询失败 | — | — |

---

## 操作危险性分级

判定依据全部来自代码路径，不靠推测。核心结论：**这个窗口里会写到「官方工具读的配置」的操作只有一个 —— 账号菜单里的「让本机用它」（`use`）。其余所有操作都只写 leeway 自己的文件（`~/.config/leeway/config.json`、`~/.config/leeway/gui.json`、`~/.cache/leeway/`、`~/.local/.../sessions` 一类由 `auth.SessionDir()` 决定的目录）。**

### 甲 · 只读（不写任何本机状态）

| 操作 | 入口 file:line | 证据 |
| --- | --- | --- |
| 刷新 / 立刻刷新 / 刷新看看 | page_provider.go:37、settingsview.go:321、page_start.go:71 | `a.Refresh(true)` → 只跑探测；凭据只读 |
| 详情 / 返回总览 / 切换设置分页 | page_provider.go:34、page_start.go:80、settingsview.go:81 | `a.navigate(...)`，只改内存与 `gui.json` 的 `LastPage` |
| 复制当前 JSON / 复制路径 | page_provider.go:41、settingsview.go:314 | 只写系统剪贴板 |
| 打开配置目录 / 打开（文件行） | page_provider.go:52、settingsview.go:304 | `mygo.Shell.ShowItemInFolder`，只在访达里选中 |
| 查看凭据来源行 / 状态胶囊 / 走势图 | page_provider.go:130-139、238-265 | 只渲染 `credentialInfo` 的结果，不读凭据内容 |
| 从浏览器导入 cookie（读的部分） | prompt.go:78 → ops.go:153 | `auth.Import` 读浏览器 cookie 库；**读**本身不改浏览器 |

注意：`刷新` 与「从浏览器导入」会**读**浏览器 cookie 库（`auth.Import`），并把 cookie header 写进 leeway 自己的缓存目录（`auth.SaveCookieCache`）。这不改官方工具，但也不是纯粹的零写入；当前文案没有提到「会读你的浏览器」。

### 乙 · 只写 leeway 自己的文件（不动官方工具）

| 操作 | 入口 file:line | 写到哪里 | 当前文案是否说清 |
| --- | --- | --- | --- |
| 启用 / 停用服务 | page_provider.go:67/99/101、page_start.go:67、settingsview.go:256 | `config.json` 的 `enabled` | 说清了「不参与探测」，但没说是写 leeway 自己的配置（不必说，用户不关心） |
| 添加账号 | page_provider.go:45 | `config.json` 加一个空账号 | 说清了「只是个标签」（promptview.go:187） |
| 填 / 粘贴 API 密钥 | view.go:575 → promptview.go:136 | `config.json`（0600） | 说清了「只写 config.json，不打印不上传」，较好 |
| 从浏览器导入 / 手动粘贴 cookie | view.go:576/577 → promptview.go:160 | leeway 缓存目录 + `config.json` | 说清了「只存在你自己的缓存目录」 |
| 导入登录文件 | view.go:578 → promptview.go:176 | `auth.SessionDir()` 下一份副本 | **说得最好的一条**：「快照到自己的会话目录，之后切号不会动原文件」 |
| 用官方 CLI 登录 | view.go:579 → ops.go:368-536 | 私有 HOME 下的官方 CLI 会话，落在 leeway 会话目录 | 弹窗底部（promptview.go:229）说清了「不会覆盖本机官方客户端的登录」，但**引导页每行的按钮旁没有这句**，用户要在点进去之后才看到 |
| 退出登录 / 删除 API key | view.go:540/548/552 → ops.go:765 | leeway 会话目录 + `config.json`；`logoutIsolated` 还有 `underDir` 校验拒绝越界删除 | 确认框（prompt.go:281/284）说清了；**但菜单项标签「退出登录」本身会让人以为要退出官方工具** |
| 清除缓存 | settingsview.go:318 → prompt.go:314 | leeway 缓存目录 | 说清了「密钥和账号不受影响」 |
| 主题 / 自动刷新 / 超时 / 历史条数 / 两个开关 | settingsview.go:102-131、154-206、269 | `gui.json` | 这些确实是界面偏好，文案不必解释 |

### 丙 · 会改本机非 leeway 状态（需要用户明确知情）

| 操作 | 入口 file:line | 改什么 | 当前文案是否说清 |
| --- | --- | --- | --- |
| **让本机用它（同步到官方工具）** | view.go:530 / page_matrix.go:128 → prompt.go:267 → ops.go:550 | cursor：`switcher.OfficialCursorTargets()`（macOS keychain 项、Cursor IDE 的 `state.vscdb`、`~/.cursor/auth.json`，存在哪个写哪个）；grok：官方 `~/.grok/auth.json`；kimi：官方 `kimi-code.json`。**codex 不在菜单里**（`canSwitchTo` 只放 cursor/grok/kimi） | **最危险且最没说清。** 菜单项「让本机用它」看不出会写官方配置；确认框（prompt.go:268）说「会写给本机官方客户端」「当前登录会被快照」，但：① 没列出会覆盖哪些位置；② 「快照成 leeway 账号」是内部说法；③ 详情页那句讲作用域的说明（page_provider.go:92）只在账号数 > 1 时出现。写入前会先备份当前登录（`SnapshotCursorIfNeeded`），读不到当前登录时**主动放弃**（switcher.go:306「已放弃切换（否则会丢当前登录）」），这一点也没写进确认框 |
| 开机自动启动 | settingsview.go:139 → app.go / mygo | macOS 登录项（系统级） | 说清了「需要打包后的 .app」，但没说这是改系统登录项 |

### 丁 · 危险但没有文案保护的两点

1. **cursor 切换要求先退出 Cursor IDE**：`switcher.CheckCursorIDE()` 会在 Cursor 运行时阻断（ops.go:620），但确认框（prompt.go:265-273）**完全没有提**「先退出 Cursor」。用户点「切换」后才在 toast 里看到失败原因 —— 这是典型的「先制造疑问，再给答案」。
2. **不可逆分支只出现在成功 toast 里（仅 cursor）**：cursor 切换时如果原登录读不出来（`res.Unsaved`，只有 cursor 这条路径会产生），切换后原登录无法恢复，警告是 app.go:664 的「原登录不在 leeway 台账里，切走之后只剩目标号」—— 它出现在**操作完成之后**的 toast 里，而不是操作之前的确认框里。这是本范围内唯一真正不可逆的路径，保护却放在事后。（grok/kimi 没有这个分支：grok 走 `mergeGrokAuth` 合并写，kimi 是整文件 `writeFileAtomic`。）

---

## 不确定清单

以下条目我没有靠代码百分之百确认，按要求单列，不硬编。

1. **「让本机用它」的推荐新名**：术语草案建议「同步到官方工具」，我也倾向它，但「同步」在中文里偏「双向/云端」，而实际是单向覆盖写入。是否改成「改用这个账号（会改官方配置）」这类更直白的名字，需要产品决定。未定。
2. **`page_provider.go:92` 的显示条件**：代码是 `if len(v.Accounts) > 1`，我按字面理解为「只有多账号时才显示这句说明」。如果 `a.providers()` 对单账号 provider 也会塞入多个 `accountView`，则条件会意外成立 —— 我没有逐条追 `providerView` 的账号填充逻辑，**不确定**。
3. **「自动刷新读一分钟内的缓存」的一分钟属于谁**：`settingsview.go:102` 这么说，但我在本次范围内只确认了 cookie 缓存有 `auth.ImportTTL = 12 * time.Hour`（auth.go:24），没有找到「1 分钟」的常量或 TTL。这一分钟可能是 `internal/fetch` 里的结果缓存。**不确定**该说法是否准确、是否该保留。
4. **「探测超时」的单位与上限**：`ui.NumberInput(c, &secs, 5, 300, 5)` 表明范围是 5–300 秒、步长 5。文案「探测超时（秒）」没有告诉用户合法范围；是否要在描述里写「5–300 秒」**不确定**（取决于产品是否希望暴露）。
5. **「配置里的 API key」（app.go:562）到底指哪一级**：从 `a.cfg.APIKey(p.ID())` 看是 provider 级。若某个 provider 还有别的层级，说法会不准确。**不确定**。
6. **`退出登录` 对 cursor 是否真的完全不碰官方 keychain**：`logoutAccount` 对 cursor 走 `auth.RemoveCursorHome(name)`（ops.go:774），删的是 leeway 会话目录下的私有 HOME。我据此判断不动官方 keychain，但没有读 `RemoveCursorHome` 的实现来排除它顺手清 keychain。**不确定**（判断倾向于「不动」）。
7. **grok 写回是否只覆盖账号段**：`ApplyGrok` → `mergeGrokAuth(dest, raw)`（switcher.go:390-406）会读现有官方文件再合并，说明不是整文件覆盖。合并的粒度（是否保留其它账号）我没有逐行读 `mergeGrokAuth`。**不确定**；这会直接影响确认框该怎么描述后果。
8. **`settingsview.go` 的「关于」卡片与侧栏「关于」页内容重复**：设置页 `settingsData` 里的「关于」只有版本/界面两行，侧栏 `aboutPage` 还有路径与探测方式说明。是否是刻意精简、还是漏了路径，**不确定**（也可能是有意避免在设置里暴露路径）。
9. **`periodText` 的 `""` 回退值「费用」**：我不确定 `snap.Cost.Period` 为空是正常情况还是异常情况，因此不敢断言「费用」这个标签一定会出现在界面上。**不确定**。

---

## 这一段文案的问题概括

这一块的文案，**「说得清」和「说不清」的两极分化非常明显，而且分界线恰好落在最该说清的地方**。好的那一极：导入登录文件（promptview.go:176「之后切号不会动原文件」）、删除密钥（prompt.go:284「本机其它地方不受影响」）、清除缓存（settingsview.go:325）都明确写了「不动什么」——说明作者知道用户怕的是什么。坏的那一极：**唯一真正会写官方工具配置的操作「让本机用它」，名字看不出会改官方配置，确认框用「快照成 leeway 账号」「本机官方客户端」这类内部说法，而且连「先退出 Cursor 才能切」都不提**；同时「退出登录」这个名字又反过来让人以为会退出官方工具，正好把恐惧装反了。第二类问题是**同一件事有太多套说法**：「刷新 / 刷新看看 / 立刻刷新」「探测 / 查询 / 答上来」「凭据 / 会话 / 登录态 / 令牌」「停用 / 不参与探测 / 已停用 / 不会参与探测」「会话 / 会话目录 / 会话文件 / 登录」「详情 / 打开 / 打开配置目录」——违反《写给大家看的设计书》的「重复」原则，用户每换一页就要重新学一遍词。第三类是**术语密度**：provider、JSON、CLI、cookie header、Profile、HOME、0600、点阵、快照、切号、台账、探测，这些词在同一个窗口里混着出现，而目标用户只是「想看看我订的那家 AI 还剩多少额度」。建议的收口顺序是：先定一份术语表（结论已收进 `docs/UI-COPY-REWRITE.md` §1，本报告的建议已对齐它），再改三个高危句子（「让本机用它」的菜单名与确认框、「退出登录」的标签、引导页首段），最后做一轮同义词合并；控件名称统一压到 2–4 个汉字，说明句统一用「会/不会动官方工具」这一句式收尾。
