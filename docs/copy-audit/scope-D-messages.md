# 文案清点 · 范围 D：弹窗 / 确认框 / 输入框 / 吐司 / 错误提示 / 朗读文本

只读分析，未改动任何代码。清点文件：`internal/gui/prompt.go`、`internal/gui/promptview.go`、`internal/gui/ops.go`、`internal/gui/app.go`、`internal/gui/theme.go`。

> 行号口径：`git status` 显示 `internal/gui/*.go` 当前有未提交改动（一次进行中的界面改版），本表按**当前工作区**的文件内容逐行核对，每条原文都用脚本比对过源文件对应行，无改写。若这些改动被回滚或提交，行号会漂移，原文案本身不变。

**条数统计：共 122 条**

| 文件 | 条数 | 说明 |
| --- | --- | --- |
| `internal/gui/prompt.go` | 21 | 弹窗标题（6 条）、吐司（5 条）、确认框（9 条）、成功后兜底（1 条） |
| `internal/gui/promptview.go` | 34 | 表单弹窗标题/副标题/按钮/字段名/提示/占位符、确认框次按钮、原生文件对话框 |
| `internal/gui/ops.go` | 38 | 错误文案 26 条、成功/日志 11 条、`displayAccount` 的 `default` 1 条 |
| `internal/gui/app.go` | 29 | 吐司 11 条、朗读 6 条、系统通知 2 条、凭据来源标签 9 条、stderr 1 条 |
| `internal/gui/theme.go` | 0 | 只有注释，没有面向用户的文字（`chrome`/`body`/`muted`/`pill` 都只是排版函数，文字由调用方给） |

**朗读（Announce）与吐司的关系（读代码得到，非猜测）**：`app.go:768` 的 `say()` 把文字压进 `a.spoken`，`view.go:117-120` 逐条交给 `c.Announce`；`a.toast` 在 `view.go:122-128` 走 `c.AddToast(ui.Toast{Title: a.toast})`，MyGo 的 toast 自己会向辅助技术播报（`gui_test.go:1275` 断言了这一点）。所以**吐司文案 = 朗读文案**，本表里的吐司条目同时是无障碍朗读文本。

**失败文案的统一加工规则**：`view.go:434` 的 `failureText(sentence, message)` 只在 message 含汉字时拼成 `句子 · 消息`；不含汉字（库的英文错误）时只返回句子，消息被丢掉。页面用第二个返回值做 tooltip（`view.go:450`），但 `app.go:656/682/696/711/724` 全部写成 `a.toast, _ = failureText(...)`，**第二个返回值被丢弃**——库错误在吐司里连 tooltip 都没有，只剩"切换失败"四个字。这是本块最系统的一个问题。

---

## 一、全量对照表

字数口径：**标题 / 名称 / 按钮 ≤ 4 个汉字，正文 / 提示 ≤ 20 个汉字**；`%s`、`%d` 这类插值（家名、账号名、路径、条数）和 `leeway`、`cursor` 这类品牌词不计入汉字数。凡标题必须同时点名 provider 和账号、4 字装不下的，我把对象移进正文，标题只留动作。

### 1.1 `internal/gui/prompt.go`（弹窗标题 / 吐司 / 确认框）

| file:line | 原文案 | 位置/控件 | 触发条件 | 功能含义 | 建议新文案（标题/正文） | 建议按钮文字 | 备注 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| internal/gui/prompt.go:72 | `"登录 " + providerID` | `prompt.title`，填 API key 弹窗 | 点账号菜单「粘贴 API key…」→ `openAPIKey` | 记录这张表单是给谁登录的 | 标题：`填密钥` | — | **实际不显示**：`promptview.go:101-109` 的副标题在 `provider != ""` 时优先用 `providerName()`，只有 provider 为空才回落到 `title`；这 6 条 `title` 都是死文案 |
| internal/gui/prompt.go:83 | `"导入 " + providerID + " 的浏览器 cookie"` | `prompt.title`，浏览器导入弹窗 | 菜单「从浏览器导入 cookie…」→ `openCookieImport` | 同上 | 标题：`读浏览器` | — | 同上，不显示 |
| internal/gui/prompt.go:97 | `"粘贴 " + providerID + " 的 cookie header"` | `prompt.title`，手动 cookie 弹窗 | 菜单「粘贴 cookie header…」→ `openManualCookie` | 同上 | 标题：`手动粘贴` | — | 同上，不显示 |
| internal/gui/prompt.go:105 | `"导入 " + providerID + " 的会话文件"` | `prompt.title`，导入会话文件弹窗 | 菜单「导入会话文件…」→ `openSessionFile` | 同上 | 标题：`导入文件` | — | 同上，不显示 |
| internal/gui/prompt.go:127 | `"给 " + providerID + " 加一个账号"` | `prompt.title`，新建账号弹窗 | 账号菜单「添加账号…」→ `openNewAccount` | 同上 | 标题：`新建账号` | — | 同上，不显示；"加一个账号"是口语但语序绕 |
| internal/gui/prompt.go:136 | `providerID + " 官方登录"` | `prompt.title`，官方 CLI 登录弹窗 | 菜单「用官方 CLI 登录…」→ `openCLI` | 同上 | 标题：`官方登录` | — | 同上，不显示 |
| internal/gui/prompt.go:182 | `"保持原来的 API key"` | 吐司（主窗口） | 账号已有 key（`keepKey`）时输入框留空并点保存 | 告诉用户没清掉旧 key | 正文：`留空，原来的密钥不变` | — | 省略主语，读起来像命令而非结果 |
| internal/gui/prompt.go:194 | `"已保存 " + providerID + " 的 API key"` | 吐司（主窗口） | `loginAPIKey` 成功返回后 `afterCredentialChange` | 报告成功并触发重新探测 | 正文：`密钥已存好` | — | providerID 是裸 id（`opencode-go`），页面已显示是哪家，冗余 |
| internal/gui/prompt.go:220 | `"已保存手动 cookie"` | 吐司（主窗口） | `saveManualCookie` 成功 | 报告成功并重新探测 | 正文：`粘贴的登录信息已存好` | — | "手动 cookie"是内部说法 |
| internal/gui/prompt.go:244 | `fmt.Sprintf("已添加账号 %s（%s）", name, providerID)` | 吐司（主窗口） | `addAccount` 成功（同步，无 goroutine） | 报告新账号已建 | 正文：`已新建账号 %s` | — | 括号里的 providerID 是裸 id；账号名是用户输入，可能很长 |
| internal/gui/prompt.go:256 | `"已更新凭据"` | 吐司（主窗口）兜底 | `afterCredentialChange("")` | 兜底成功文案 | 正文：`登录信息已更新` | — | **实际不可达**：三处调用点都传了非空消息（`importCookies`/`importAuthFile` 成功必返回非空），属防御分支 |
| internal/gui/prompt.go:267 | `fmt.Sprintf("让本机 %s 改用 %s？", providerID, displayAccount(account))` | 确认框标题（`askUse`） | 账号菜单出现「让本机用它」且 `accountSwitchReady` 为真，点它 | 问是否把本机官方客户端切到这个号 | 标题：`本机改用 %s？` | — | providerID 是裸 id，和弹窗副标题用显示名（`providerName`）不一致；标题走 `chrome()`，单行、超长会被裁 |
| internal/gui/prompt.go:268 | `"leeway 会把该账号的会话写给本机官方客户端；本机当前登录会被快照成 leeway 账号。"` | 确认框正文（`askUse`） | 同上 | 说明会发生什么 | 正文：`本机会改用这个号，现在的登录先存一份` | — | "写给""快照"是内部黑话；**没提前说**当前登录若不在 leeway 台账里就会丢，这句只在事后吐司（`app.go:664`）出现 |
| internal/gui/prompt.go:269 | `"切换"` | 确认框主按钮（`askUse`） | 同上 | 执行 `a.Use` | — | `切换本机` | "切换"没说是切换什么；且这颗按钮被涂成 critical 红（`promptview.go:271-273`），和删除类操作同一视觉等级 |
| internal/gui/prompt.go:280 | `fmt.Sprintf("退出 %s 账号 %s？", providerID, displayAccount(account))` | 确认框标题（`askLogout`，有会话） | 账号菜单点「退出登录」，且 `accountUsesSession` 为真（官方 CLI 类 provider，或有 cookie 的号） | 问是否删除该号在 leeway 里的登录 | 标题：`删除登录？` | — | 标题说"退出"、正文说"删除"，同一件事两个动词；`default` 账号会显示成"账号 default"（见 ops.go:27） |
| internal/gui/prompt.go:281 | `"会删除该账号在 leeway 里保存的私有会话、cookie 与配置里的 key，不影响本机官方客户端里的登录（cursor/grok 还会清掉私有 HOME）。"` | 确认框正文（`askLogout`，有会话） | 同上 | 说明删什么、不删什么 | 正文：`%s 账号 %s 的登录会被删掉，删了要重登` | — | 前半句的"不影响本机官方客户端"和括号里"还会清掉私有 HOME"自相矛盾；"私有 HOME""key"是黑话；**不可逆**但没写"删了要重新登录" |
| internal/gui/prompt.go:283 | `fmt.Sprintf("删除 %s 账号 %s 的 API key？", providerID, displayAccount(account))` | 确认框标题（`askLogout`，只有 key） | 账号菜单点「删除 API key」，且 `accountUsesSession` 为假（纯 key/cookie 类且无 cookie） | 问是否删掉存的 key | 标题：`删除密钥？` | — | 和上一条的"退出"是两套句式，同类操作没统一 |
| internal/gui/prompt.go:284 | `"只删掉 leeway 里存的这个 key，本机其它地方不受影响。"` | 确认框正文（`askLogout`，只有 key） | 同上 | 说明影响范围 | 正文：`%s 账号 %s 存的密钥会被删掉，别处不动` | — | **不可逆**：删了得重新粘贴，正文没提醒 |
| internal/gui/prompt.go:313 | `"清除 cookie 缓存？"` | 确认框标题（`askClearCache`） | 设置窗口 → 数据 → 「清除 cookie 缓存」 | 问是否丢掉缓存 | 标题：`清除缓存？` | — | "cookie 缓存"是黑话；这是四个确认框里唯一真正可逆的 |
| internal/gui/prompt.go:314 | `"下一次探测会重新从浏览器读一次 cookie。config.json 里的密钥和账号不受影响。"` | 确认框正文（`askClearCache`） | 同上 | 说明后果 | 正文：`下次探测会重读浏览器，密钥和账号不动` | — | 四条正文里写得最清楚的一条 |
| internal/gui/prompt.go:315 | `"清除"` | 确认框主按钮（`askClearCache`） | 同上 | 执行 `a.ClearCache` | — | `清除缓存` | 只有两个字的动词，和正文"下一次探测会重读"对应得上 |

### 1.2 `internal/gui/promptview.go`（表单弹窗、确认框、原生对话框）

| file:line | 原文案 | 位置/控件 | 触发条件 | 功能含义 | 建议新文案（标题/正文） | 建议按钮文字 | 备注 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| internal/gui/promptview.go:65 | `"保存"` | 弹窗主按钮 | 弹窗类型不是官方 CLI（key / 浏览器导入 / 手动 cookie / 会话文件 / 新建账号） | 提交表单 | — | 按类型：`存密钥`、`开始导入`、`存起来`、`导入`、`建账号` | 五张表单共用一个"保存"，用户看不出点下去到底做什么 |
| internal/gui/promptview.go:67 | `"完成"` | 弹窗主按钮 | 官方 CLI 弹窗且 `done` 为真 | 关掉已完成的登录弹窗 | — | `完成` | 语义是"关闭"，和左侧 `关闭` 重复 |
| internal/gui/promptview.go:85 | `"粘贴 API key"` | 弹窗标题（`chrome`，单行） | 弹窗类型 `promptAPIKey` | 命名这张表单 | 标题：`填密钥` | — | "API key"是术语；4 字上限下用"填密钥" |
| internal/gui/promptview.go:87 | `"从浏览器导入 cookie"` | 弹窗标题 | `promptCookieImport` | 命名 | 标题：`读浏览器` | — | 4 字塞不下"从浏览器导入登录态"，退一步用"读浏览器"，读的是什么由字段名和副标题交代 |
| internal/gui/promptview.go:89 | `"粘贴 cookie header"` | 弹窗标题 | `promptManualCookie` | 命名 | 标题：`手动粘贴` | — | "cookie header"是术语，字段名里已重复一次 |
| internal/gui/promptview.go:91 | `"导入会话文件"` | 弹窗标题 | `promptSessionFile` | 命名 | 标题：`导入文件` | — | "会话文件"是内部词 |
| internal/gui/promptview.go:93 | `"添加账号"` | 弹窗标题 | `promptNewAccount` | 命名 | 标题：`新建账号` | — | 已是 4 字，只把"添加"换成更日常的"新建" |
| internal/gui/promptview.go:95 | `"用官方 CLI 登录"` | 弹窗标题 | `promptCLI` | 命名 | 标题：`官方登录` | — | "CLI"是术语 |
| internal/gui/promptview.go:97 | `"登录"` | 弹窗标题兜底 | `promptHeader()` 的 default，只有 `kind == promptNone` 才走到 | 兜底 | 标题：`登录` | — | **不可达**：弹窗只在 `active()`（kind != none）时绘制 |
| internal/gui/promptview.go:107 | `name + " · 账号 " + a.prompt.account` | 弹窗副标题（`muted`） | 弹窗的 `account` 非空且不是 `default`（即从具名账号行打开） | 说明这张表单是给哪个号的 | 正文：`%s · 账号 %s` | — | 用的是 provider **显示名**（`providerName`），和确认框标题用裸 id 不一致；账号名是原样拼接 |
| internal/gui/promptview.go:115 | `"停止"` | 弹窗次按钮 | 官方 CLI 弹窗且 `running` | 取消正在跑的登录进程 | — | `停止登录` | "停止"停的是什么不明确 |
| internal/gui/promptview.go:117 | `"关闭"` | 弹窗次按钮 | 官方 CLI 弹窗且已结束 | 关掉弹窗 | — | `关闭` | 可以 |
| internal/gui/promptview.go:119 | `"取消"` | 弹窗次按钮 | 其余四种弹窗 | 放弃填写 | — | `取消` | 表单弹窗用"取消"没问题；**确认框**的"取消"才是问题（见 1.2 末行） |
| internal/gui/promptview.go:136 | `"key 会写进 ~/.config/leeway/config.json（0600），不打印、不上传。"` | 表单字段提示（`formRow` 的 hint） | `promptAPIKey` 且该号还没有 key | 说明密钥存哪、不外传 | 正文：`只存在你自己的电脑上` | — | 28 字超 20；"0600""不打印、不上传"是实现细节；`~/.config/...` 路径对普通用户无意义 |
| internal/gui/promptview.go:138 | `"已经有一把 key：留空就是保持不变，填新的会覆盖。"` | 表单字段提示 | `promptAPIKey` 且该号已有 key | 说明留空/填新的两种结果 | 正文：`留空不改，填新的会覆盖` | — | "一把 key"口语但和别处"API key"不统一 |
| internal/gui/promptview.go:150 | `"显示明文"` | 复选框标签 | `promptAPIKey` 弹窗 | 切换密码框为明文 | — | `显示明文` | 已是 4 字、无术语，保留 |
| internal/gui/promptview.go:156 | `"浏览器"` | 表单字段名 | `promptCookieImport` 弹窗 | 输入浏览器名 | — | — | 保留 |
| internal/gui/promptview.go:159 | `"Profile（可留空）"` | 表单字段名 | `promptCookieImport` 弹窗 | 输入浏览器 profile | 名称：`配置名` | — | "Profile"是英文术语且 4 字上限外；"可留空"应下沉到提示 |
| internal/gui/promptview.go:160 | `"leeway 从浏览器的 cookie 库里读出这个 provider 的登录态，只存在你自己的缓存目录。"` | 表单字段提示 | 同上 | 说明会读浏览器、只存本地 | 正文：`从浏览器读登录，只存在本机` | — | 40 字，严重超 20；"登录态""缓存目录"是黑话 |
| internal/gui/promptview.go:167 | `"适合浏览器导入不成功的场景：从开发者工具里复制 Cookie 请求头。这个框是明文，旁边有人或录屏时注意。"` | 表单字段提示 | `promptManualCookie` 弹窗 | 说明用途和明文风险 | 正文：`浏览器读不到时用；这里是明文` | — | 48 字；"开发者工具""Cookie 请求头"是术语；明文提醒是好内容，但埋在一长句里 |
| internal/gui/promptview.go:170 | `"name=value; name2=value2（curl -b 或 Cookie: 片段也行）"` | 输入框占位符（TextArea） | 同上 | 举例粘贴格式 | 正文：`粘贴浏览器里那一整行` | — | `name=value`、`curl -b` 是术语；占位符太长会被截 |
| internal/gui/promptview.go:175 | `"会话文件"` | 表单字段名 | `promptSessionFile` 弹窗 | 选择会话文件 | 名称：`登录文件` | — | 与弹窗标题（`导入文件`）统一 |
| internal/gui/promptview.go:176 | `"leeway 把这份文件快照到自己的会话目录，之后切号不会动原文件。"` | 表单字段提示 | 同上 | 说明复制一份、不动原文件 | 正文：`复制一份用，原文件不动` | — | "快照""切号"是黑话 |
| internal/gui/promptview.go:179 | `"选择…"` | 行内按钮 | `promptSessionFile` 弹窗 | 打开系统文件对话框 | — | `选文件` | 省略号是 macOS 习惯，但"选择"不如"选文件"具体 |
| internal/gui/promptview.go:186 | `"账号名"` | 表单字段名 | `promptNewAccount` 弹窗 | 输入新账号的标签 | — | — | 保留 |
| internal/gui/promptview.go:187 | `"只是个标签，之后可以给它单独登录一份凭据，再用「让本机用它」切换。"` | 表单字段提示 | 同上 | 说明名字只是标签、后续怎么用 | 正文：`只是名字，之后可以单独登录` | — | 37 字；"凭据"是术语；引用了另一个菜单项名（跨界面耦合，改名要一起改） |
| internal/gui/promptview.go:188 | `"work / home / 邮箱"` | 输入框占位符 | 同上 | 举例账号名 | 正文：`工作 / 家里 / 邮箱` | — | 中英混排举例，和全中文界面不统一 |
| internal/gui/promptview.go:203 | `"等待官方 CLI 完成登录…"` | 状态行（`muted` + Spinner） | 官方 CLI 弹窗且 `running` | 说明正在等 | 正文：`正在等官方登录完成` | — | 已经不错；"CLI"是术语 |
| internal/gui/promptview.go:205 | `"登录完成"` | 状态行（`body`，成功色） | 官方 CLI 弹窗且 `done && err == ""` | 报告成功 | 正文：`登录好了` | — | 与 `prompt.go:194` 的"已保存…"句式不一致（一个"完成"一个"已…"） |
| internal/gui/promptview.go:214 | `"（官方 CLI 还没有输出）"` | 日志面板占位文字（mono） | 官方 CLI 弹窗且日志 trim 后为空 | 说明面板为什么是空的 | 正文：`官方登录还没输出内容` | — | 括号包裹，和其它占位文案句式不统一 |
| internal/gui/promptview.go:229 | `"登录在 leeway 自己的 HOME 里进行，不会覆盖你本机官方客户端的登录。"` | 面板底部提示（`unconfiguredLine`） | 官方 CLI 弹窗，始终显示 | 让用户放心不会动本机登录 | 正文：`登录只存 leeway，不动你的官方客户端` | — | 36 字；"HOME"是术语（大写更重）；这条和 `prompt.go:281`"还会清掉私有 HOME"互相打架 |
| internal/gui/promptview.go:239 | `"选择会话文件"` | 系统文件对话框标题（`mygo.Dialog.Open`） | `promptSessionFile` 弹窗点「选择…」 | 命名原生对话框 | 标题：`选文件` | — | 原生对话框标题；与字段名（`登录文件`）统一 |
| internal/gui/promptview.go:241 | `"会话"` | 文件类型过滤器名 | 同上 | 过滤 `.json` | 名称：`登录文件` | — | 过滤器名和字段名不一致 |
| internal/gui/promptview.go:264 | `"取消"` | 确认框次按钮（`confirmSheet`） | 任一确认框打开时 | 什么都不做、关掉对话框 | — | 按类型：`先不切`、`不删了`、`不清了` | 《Don't Make Me Think》：确认框的"取消"要说清"不做什么"；这里 4 个确认框共用"取消" |

### 1.3 `internal/gui/ops.go`（后台操作的成功 / 失败文案）

| file:line | 原文案 | 位置/控件 | 触发条件 | 功能含义 | 建议新文案（标题/正文） | 建议按钮文字 | 备注 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| internal/gui/ops.go:27 | `"default"` | `displayAccount()` 返回值，出现在确认框标题和吐司里 | 账号名为空时 | 表示"默认账号" | 名称：`默认` | — | 中文界面里唯一的英文标签，会出现在"退出 kimi 账号 default？"这种句子里 |
| internal/gui/ops.go:139 | `"API key 为空"` | 错误 → 弹窗错误行（`promptview.go:58` 的 `errorLine`） | 该号没有已存 key，用户直接点保存 | 拦下空提交 | 正文：`密钥是空的，先填上` | — | 说了原因没说下一步；且空输入本可以就地禁用按钮 |
| internal/gui/ops.go:163 | `fmt.Sprintf("缓存里的 cookie 还新（%s，%s 导入），跳过", cc.ImportedAt.Local().Format("01-02 15:04"), cc.Browser)` | 成功吐司 | `importCookies(force=false)` 且缓存未过期 | 说明复用了缓存 | 正文：`缓存还新，跳过导入` | — | **GUI 不可达**：唯一调用点 `prompt.go:200` 固定传 `force=true` |
| internal/gui/ops.go:182 | `fmt.Sprintf("已从 %s 导入 %d 条 cookie", browser, n)` | 成功吐司 | 浏览器 cookie 导入成功 | 报告条数 | 正文：`已从 %s 导入 %d 条登录信息` | — | "cookie"是术语；浏览器名是英文原样（chrome/safari） |
| internal/gui/ops.go:193 | `"cookie header 里没有 name=value"` | 错误 → 弹窗错误行 | 粘贴内容里没有 `=` | 拦下无效粘贴 | 正文：`没找到登录信息，请复制完整的一行` | — | 术语；说了原因没说下一步 |
| internal/gui/ops.go:243 | `"账号名不能为空"` | 错误 → 弹窗错误行 | 新建账号弹窗里名字为空 | 拦下空名字 | 正文：`账号名是空的，先起个名` | — | 说了原因没说下一步 |
| internal/gui/ops.go:283 | `"没有选择文件"` | 错误 → 弹窗错误行 | 会话文件弹窗路径为空 | 拦下空路径 | 正文：`还没选文件，点右边的选文件` | — | 说了原因没说下一步 |
| internal/gui/ops.go:297 | `"这不是完整的官方 grok 会话（缺 profile/email 字段），use 会拒绝写回"` | 错误 → 弹窗错误行 | 导入 grok 文件时 `grok.HasOfficialShape` 为假 | 说明文件不合格 | 正文：`文件缺官方字段，请重新登录 grok` | — | "use""写回"是内部黑话（`use` 还是 CLI 子命令名）；原因有、下一步没有 |
| internal/gui/ops.go:323 | `fmt.Errorf("%s 的会话不是文件导入的", providerID)` | 错误 → 弹窗错误行 | `importAuthFile` 的 default 分支 | 说明这家不支持文件导入 | 正文：`这家不支持导入文件，请改用登录` | — | **GUI 不可达**：`openSessionFile` 只对 grok/kimi/codex/cursor 出现（`view.go:598`），四种都有分支 |
| internal/gui/ops.go:326 | `fmt.Errorf("这份文件不是可用的 %s 会话", providerID)` | 错误 → 弹窗错误行 | 文件能读但 `sessionChecks` 校验不过 | 说明文件内容不对 | 正文：`不是 %s 的登录文件，请换一个` | — | 原因偏笼统；没说什么样的才算合格 |
| internal/gui/ops.go:344 | `fmt.Sprintf("已快照 %s 账号 %s", providerID, displayAccount(account))` | 成功吐司 | 会话文件导入成功 | 报告已存下 | 正文：`已存下 %s 账号 %s 的登录` | — | "快照"是黑话；providerID 是裸 id |
| internal/gui/ops.go:380 | `fmt.Errorf("%s 用 API key 登录", providerID)` | 错误 → 弹窗错误行 | `officialLogin` 的 default 分支 | 说明这家不用 CLI | 正文：`%s 要填密钥登录，不用官方 CLI` | — | **GUI 不可达**：`openCLI` 只对声明 `OfficialCLI` 的四家出现（`view.go:586`），而 `officialLogin` 恰好覆盖这四家 |
| internal/gui/ops.go:387 | `"没找到 cursor-agent CLI，先安装 https://cursor.com/cli"` | 错误 → 弹窗错误行 | `exec.LookPath("cursor-agent")` 失败 | 说明缺命令行工具并给安装地址 | 正文：`没装 cursor-agent，先装好再来` | — | 原因+下一步齐全，是本块最好的一条；URL 混在句子里不可点，建议单独一行 |
| internal/gui/ops.go:411 | `"登录似乎没完成：官方 CLI 没写出令牌"` | 错误 → 弹窗错误行 | cursor CLI 退出码 0 但读不到 access token | 说明登录没真正完成 | 正文：`官方登录没写出结果，请再试一次` | — | "令牌"是术语；没说下一步 |
| internal/gui/ops.go:438 | `"\n已登录 cursor 账号 " + name + "\n"` | 登录弹窗内的日志面板（mono，可选中） | cursor 官方登录成功 | 在日志里写一行结果 | 正文：`cursor 账号 %s 已登录` | — | 与另外三家的日志句式一致（好事）；`name` 可能是邮箱 |
| internal/gui/ops.go:449 | `fmt.Errorf("grok 账号 %s 已有完整官方快照；要替换请用「导入会话文件」", name)` | 错误 → 弹窗错误行 | grok 已有完整 `auth.json` 时点官方登录 | 拦下会覆盖的登录 | 正文：`%s 已有完整登录，想换请导入文件` | — | 原因+下一步齐全；"快照"是黑话；引用的菜单名与 `view.go:578` 耦合 |
| internal/gui/ops.go:453 | `"没找到 grok CLI，先安装：curl -fsSL https://x.ai/cli/install.sh \| bash"` | 错误 → 弹窗错误行 | `exec.LookPath("grok")` 失败 | 说明缺 CLI 并给安装命令 | 正文：`没装 grok CLI，先装好再来` | — | 原因+下一步齐全，但把 `curl \| bash` 塞进错误行很危险也不可复制；应改成一行可复制的命令或文档链接 |
| internal/gui/ops.go:471 | `"\n已登录 grok 账号 " + name + "\n"` | 登录弹窗日志面板 | grok 官方登录成功 | 写日志 | 正文：`grok 账号 %s 已登录` | — | 同上 |
| internal/gui/ops.go:473 | `"\n设备码会话缺官方资料字段，写回本机 grok 会被拒绝；请用「导入会话文件」快照 ~/.grok/auth.json\n"` | 登录弹窗日志面板 | grok 设备码登录成功但 `HasOfficialShape` 为假 | 提醒这份会话切不过去 | 正文：`缺官方资料，请改导入 ~/.grok/auth.json` | — | 原因+下一步齐全；"写回""快照"是黑话；含真实家目录路径 |
| internal/gui/ops.go:481 | `"没找到 codex CLI，先安装官方 Codex CLI"` | 错误 → 弹窗错误行 | `exec.LookPath("codex")` 失败 | 说明缺 CLI | 正文：`没装 codex CLI，先装好再来` | — | 说了原因，**没说怎么装**（另外三家都给了地址或命令） |
| internal/gui/ops.go:504 | `"\n已登录 codex 账号 " + name + "\n"` | 登录弹窗日志面板 | codex 官方登录成功 | 写日志 | 正文：`codex 账号 %s 已登录` | — | 同上 |
| internal/gui/ops.go:511 | `"没找到 kimi CLI；可以用「粘贴 API key」或「导入浏览器 cookie」"` | 错误 → 弹窗错误行 | `exec.LookPath("kimi")` 失败 | 说明缺 CLI 并给替代路径 | 正文：`没装 kimi CLI，可以改用填密钥` | — | 原因+替代方案齐全，且引用了界面里的菜单名（好做法） |
| internal/gui/ops.go:534 | `"\n已登录 kimi 账号 " + name + "\n"` | 登录弹窗日志面板 | kimi 官方登录成功 | 写日志 | 正文：`kimi 账号 %s 已登录` | — | 同上 |
| internal/gui/ops.go:564 | `"kimi 这个号只有 key/cookie，没有会话文件可写回官方 kimi-code.json"` | 错误 → 吐司（`app.go:656` 包一层"切换失败"） | 对只有 key/cookie 的 kimi 号点「让本机用它」 | 说明切不了 | 正文：`这个号没有登录文件，请先登录 kimi` | — | "写回""kimi-code.json"是内部细节；没说下一步 |
| internal/gui/ops.go:566 | `"codex 不写官方 ~/.codex（避免 refresh 把官方会话作废）"` | 错误 → 吐司 | 对 codex 点「让本机用它」 | 说明这是有意不支持 | 正文：`codex 不能切本机，会弄坏官方登录` | — | 其实是"设计上拒绝"而非失败，却走错误通道；"refresh""作废"是黑话 |
| internal/gui/ops.go:568 | `fmt.Errorf("%s 没有本机官方客户端可切", providerID)` | 错误 → 吐司 | `useAccount` 的 default 分支 | 说明这家没有可切的客户端 | 正文：`%s 没有官方客户端，不能切换` | — | 原因有、下一步无；GUI 侧已被 `canSwitchTo` 挡住（`view.go:629`），属兜底 |
| internal/gui/ops.go:606 | `fmt.Errorf("cursor 账号 %q 还没登录", name)` | 错误 → 吐司 | cursor 私有会话缺 access/refresh token 时点切换 | 说明得先登录 | 正文：`cursor 账号 %s 还没登录，请先登录` | — | `%q` 会输出带引号的 `"work"`，中文句子里夹英文引号；没说下一步 |
| internal/gui/ops.go:609 | `fmt.Errorf("cursor 账号 %q 的令牌已过期，重新登录", name)` | 错误 → 吐司 | JWT 过期时点切换 | 说明过期了要重登 | 正文：`cursor 账号 %s 登录过期，请重新登录` | — | 原因+下一步齐全；"令牌"是术语 |
| internal/gui/ops.go:681 | `fmt.Errorf("grok 账号 %q 还没登录", name)` | 错误 → 吐司 | grok 源文件不存在时点切换 | 说明得先登录 | 正文：`grok 账号 %s 还没登录，请先登录` | — | 同 606 |
| internal/gui/ops.go:684 | `fmt.Errorf("grok 账号 %q 不是完整官方会话，写回会逼 grok 重新网页登录；请先导入 ~/.grok/auth.json", name)` | 错误 → 吐司 | grok 会话缺官方字段时点切换 | 说明切了会掉登录，并给替代 | 正文：`%s 的登录不完整，切了要重登网页；请导入 ~/.grok/auth.json` | — | 原因+下一步齐全；"写回""逼"是黑话，`%q` 带引号；超 20 字 |
| internal/gui/ops.go:717 | `fmt.Errorf("kimi 这个号只有 key/cookie，没有会话文件可写回官方 kimi-code.json")` | 错误 → 吐司 | kimi 账号无 `AuthFile` 时点切换 | 说明切不了 | 正文：`这个号没有登录文件，请先登录 kimi` | — | 与 564 重复（同一句话两处维护） |
| internal/gui/ops.go:724 | `fmt.Errorf("kimi 账号 %q 的会话文件不在 %s", account, src)` | 错误 → 吐司 | 配置里的会话文件已被删/移走 | 说明文件找不到 | 正文：`%s 的登录文件不见了，请重新登录` | — | `%s` 是**真实绝对路径**，会进吐司（截图/录屏会带出用户名）；不说下一步；`%q` 带引号 |
| internal/gui/ops.go:731 | `fmt.Errorf("kimi 账号 %q 不是完整 CLI 会话", account)` | 错误 → 吐司 | 会话文件校验不过时点切换 | 说明文件不合格 | 正文：`%s 的登录文件不完整，请重新登录` | — | 原因偏笼统，无下一步 |
| internal/gui/ops.go:750 | `"这个号就绑在官方会话文件上，先快照一份再切"` | 错误 → 吐司 | kimi 账号的 `AuthFile` 与官方路径是同一文件时点切换 | 拦下自覆盖 | 正文：`这个号用的就是官方文件，先存一份再切` | — | 原因+下一步齐全；"快照""切"是黑话 |
| internal/gui/ops.go:781 | `fmt.Sprintf("已清除 cursor 账号 %s 的凭据", name), nil` | 成功吐司 | cursor 退出登录成功 | 报告结果 | 正文：`已删掉 cursor 账号 %s 的登录` | — | 与 862 句式不统一（"凭据" vs "私有登录"） |
| internal/gui/ops.go:809 | `fmt.Sprintf("已移除 %s 账号 %s", providerID, name)` | 成功吐司 | deepseek/opencode-go/commandcode 等退出成功 | 报告结果 | 正文：`已删掉 %s 账号 %s` | — | providerID 是裸 id；"移除"和别处"清除""退出"三套动词 |
| internal/gui/ops.go:856 | `fmt.Errorf("拒绝删除 %s：不在 leeway 会话目录里", home)` | 错误 → 吐司 | 待删目录不在会话根目录下（`underDir` 保护） | 安全拦截 | 正文：`没有删 %s：它不在 leeway 目录里` | — | 原因有、下一步无（这属于不该让用户处理的内部保护）；`%s` 是真实绝对路径 |
| internal/gui/ops.go:862 | `fmt.Sprintf("已清除 %s 账号 %s 的私有登录", providerID, name), nil` | 成功吐司 | grok/codex/kimi 退出登录成功 | 报告结果 | 正文：`已删掉 %s 账号 %s 的登录` | — | "私有登录"是内部说法 |

### 1.4 `internal/gui/app.go`（状态、吐司、朗读、错误行、凭据标签）

| file:line | 原文案 | 位置/控件 | 触发条件 | 功能含义 | 建议新文案（标题/正文） | 建议按钮文字 | 备注 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| internal/gui/app.go:201 | `fmt.Fprintf(os.Stderr, "leeway: 窗口截图失败：%v\n", err)` | stderr（`LEEWAY_GUI_CAPTURE` 自检） | 截图或写文件失败 | 给开发者报错 | 正文：`leeway: 截图没成功：%v` | — | 不进 GUI，属开发诊断；`%v` 是库错误原文 |
| internal/gui/app.go:306 | `failureText("探测失败", ferr)` | 朗读 + 系统通知正文 + 页面错误行（三处同源） | `fetch.FetchAll` 返回错误 | 报告探测失败 | 正文：`这次没查到：<原因>，稍后再试` | — | ferr 不含汉字时朗读只剩"探测失败"；原因缺失 |
| internal/gui/app.go:316 | `a.say("刷新完成：" + a.summary())` | 朗读（`c.Announce`） | 探测成功完成 | 念一句结果 | 正文：`刷新好了：%s` | — | 只在朗读里，屏幕上没有对应文字（视觉用户看不到这次成功） |
| internal/gui/app.go:327 | `fmt.Sprintf("%d 正常 · %d 失败 · %d 未配置", ok, bad, missing)` | 朗读内容 + 标题栏状态行 | 每次渲染标题栏；刷新成功时被朗读 | 一行概括本次结果 | 正文：`%d 可用 / %d 失败 / %d 没配` | — | **用词与含义不符**：`overview()` 把"有凭据但没探测过"的 provider 也算进 `ok`（`app.go:630-645` 注释自陈），说"正常"会过头；"未配置"同理 |
| internal/gui/app.go:341 | `Title: "leeway 探测失败"` | 系统通知标题 | 探测失败、`NotifyOnError` 开、窗口不在前台、系统支持通知 | 通知标题 | 标题：`没查到` | — | 正文是 `app.go:306` 的句子，所以标题重复"失败" |
| internal/gui/app.go:556 | `kind = "账号 API key"` | 账号卡片"凭据来自…"的值 | 账号级 APIKey 非空 | 说明凭据从哪来 | 名称：`账号密钥` | — | 术语；四类标签句式不统一（"账号 X"/"环境变量 X"/"配置里的 X"/"会话文件 X"） |
| internal/gui/app.go:558 | `kind = "环境变量 " + spec.EnvAPIKey[0]` | 同上 | 环境变量有值 | 说明来自环境变量 | 名称：`环境变量 %s` | — | 变量名是英文（如 `DEEPSEEK_API_KEY`），可接受 |
| internal/gui/app.go:560 | `kind = "环境变量 " + spec.EnvAuthToken[0]` | 同上 | auth token 环境变量有值 | 同上 | 名称：`环境变量 %s` | — | 同上 |
| internal/gui/app.go:562 | `kind = "配置里的 API key"` | 同上 | provider 级 APIKey 非空 | 说明来自 config.json | 名称：`配置密钥` | — | 术语 |
| internal/gui/app.go:564 | `kind = "会话文件 " + shortenPath(acc.AuthFile)` | 同上 | 账号有 `AuthFile` | 说明来自哪个文件 | 名称：`登录文件 %s` | — | 路径已 `shortenPath`（`~` 缩写），仍可能带用户名 |
| internal/gui/app.go:566 | `kind = "手动 cookie"` | 同上 | 账号有手动 cookie | 说明来自手动粘贴 | 名称：`手动粘贴` | — | 术语 |
| internal/gui/app.go:575 | `kind = "Codex CLI 会话"` | 同上 | codex 官方 CLI 的 auth.json 在磁盘上 | 说明来自官方 CLI | 名称：`Codex 官方登录` | — | 术语 |
| internal/gui/app.go:579 | `kind = "会话文件 " + shortenPath(path)` | 同上 | provider 声明的本地会话文件存在 | 同上 | 名称：`登录文件 %s` | — | 与 564 同句式（好） |
| internal/gui/app.go:586 | `kind = "cursor 私有会话"` | 同上 | cursor 私有会话可读 | 说明来自私有会话 | 名称：`cursor 登录` | — | "私有会话"是内部词 |
| internal/gui/app.go:588 | `expires = exp.Local().Format("01-02 15:04") + " 过期"` | 账号卡片到期提示 | cursor JWT 能解析出过期时间 | 提示何时过期 | 正文：`%s 到期` | — | 只给月-日 时:分，没有年份/相对时间；"过期"是状态词，放在时间后面像"过期了" |
| internal/gui/app.go:656 | `failureText("切换失败", err.Error())` | 吐司（切号失败） | `useAccount` 返回错误 | 报告切换失败 | 正文：`没能切换：<原因>，可以再试一次` | — | 第二个返回值被丢弃（`_`）→ 库英文错误连 tooltip 都没有；建议改成"原因 + 下一步" |
| internal/gui/app.go:659 | `"已切换 " + res.Provider + " 到 " + displayAccount(res.Account)` | 吐司（切号成功） | 确认框点「切换」且成功 | 报告切到了哪个号 | 正文：`本机已改用 %s` | — | `res.Provider` 是裸 id；"切换…到…"语序绕 |
| internal/gui/app.go:661 | `"（原账号已存为 " + res.SavedAs + "）"` | 吐司追加片段 | `res.SavedAs` 非空（切前快照成功） | 说明旧登录被存下了 | 正文：`（原登录已存为 %s）` | — | 与 664 是互斥的两条，用户读到的永远是其中一条 |
| internal/gui/app.go:664 | `"；原登录不在 leeway 台账里，切走之后只剩目标号"` | 吐司追加片段 | `res.Unsaved` 为真 | 警告旧登录没被存下、找不回 | 正文：`原来的登录没存过，切走后找不回` | — | **最该前置的一条**：确认框里没说，只在切完之后告知；"台账"是黑话 |
| internal/gui/app.go:667 | `"（写入 " + strings.Join(res.Targets, "、") + "）"` | 吐司追加片段 | `res.Targets` 非空 | 列出写了哪几个文件 | 正文：`（写入 %d 个文件）` | — | 多个真实绝对路径（含用户名）塞进吐司，又长又敏感；建议只给数量、详情进 tooltip |
| internal/gui/app.go:682 | `failureText("退出失败", err.Error())` | 吐司（退出登录失败） | `logoutAccount` 返回错误 | 报告失败 | 正文：`没能退出：<原因>，可以再试一次` | — | 同 656：英文详情被丢弃 |
| internal/gui/app.go:696 | `failureText("设置保存失败", err.Error())` | 吐司（设置窗口） | `settings.save()` 失败 | 报告失败 | 正文：`设置没存上：<原因>，可以再试一次` | — | 同 656；`log.Printf` 里有英文详情但用户看不到 |
| internal/gui/app.go:711 | `failureText("清除缓存失败", err.Error())` | 吐司 | `auth.RemoveCookieCacheFile()` 失败 | 报告失败 | 正文：`缓存没清掉：<原因>，可以再试一次` | — | 同 656 |
| internal/gui/app.go:714 | `"已清除 cookie 缓存"` | 吐司 | 清缓存成功 | 报告成功 | 正文：`登录缓存已清除` | — | 术语；与确认框标题"清除 cookie 缓存？"同词（一致性尚可） |
| internal/gui/app.go:724 | `failureText("保存失败", err.Error())` | 吐司（provider 启停开关） | `setProviderEnabled` 失败 | 报告失败 | 正文：`开关没存上：<原因>，可以再试一次` | — | "保存失败"和 696 的"设置保存失败"含义重叠、用户分不清是哪次保存 |
| internal/gui/app.go:778 | `return "总览"` | 朗读（`pageName`，`navigate` 调 `say`） | 切到总览页 | 念出页面名 | 正文：`总览` | — | 与侧边栏字面一致（好） |
| internal/gui/app.go:780 | `return "历史"` | 朗读 | 切到历史页 | 同上 | 正文：`历史` | — | 同上 |
| internal/gui/app.go:782 | `return "关于"` | 朗读 | 切到关于页 | 同上 | 正文：`关于` | — | 同上 |
| internal/gui/app.go:784 | `return providerName(strings.TrimPrefix(page, "provider:"))` | 朗读 | 切到某 provider 详情页 | 念出 provider 显示名 | 正文：`%s` | — | 用的是显示名（如 `Kimi Code`），英文品牌名会被念成英文，可接受 |

---

## 二、错误文案表（原因 + 下一步是否齐全）

判断标准：《Don't Make Me Think》——失败必须说清"为什么失败"和"我现在该做什么"。`✔` 齐全，`△` 只有一半，`✘` 两者都缺。

| file:line | 原文案 | 原因 | 下一步 | 判断 |
| --- | --- | --- | --- | --- |
| ops.go:139 | `API key 为空` | ✔ | ✘ | △ 空输入本可禁用按钮，不该走到报错 |
| ops.go:193 | `cookie header 里没有 name=value` | ✔ | ✘ | △ 缺"复制完整一行" |
| ops.go:243 | `账号名不能为空` | ✔ | ✘ | △ 缺"先起个名" |
| ops.go:283 | `没有选择文件` | ✔ | ✘ | △ 缺"点右边选文件" |
| ops.go:297 | `这不是完整的官方 grok 会话（缺 profile/email 字段），use 会拒绝写回` | ✔ | ✘ | △ 说了后果，没给"用官方 grok 登录一次" |
| ops.go:323 | `%s 的会话不是文件导入的` | △ | ✘ | ✘ 不可达；措辞像内部断言 |
| ops.go:326 | `这份文件不是可用的 %s 会话` | △（只说"不可用"） | ✘ | △ 应说"不是这家官方的登录文件" |
| ops.go:380 | `%s 用 API key 登录` | △ | ✘ | ✘ 不可达；把"该怎么登录"写成了错误 |
| ops.go:387 | `没找到 cursor-agent CLI，先安装 https://cursor.com/cli` | ✔ | ✔ | ✔ 本块最好的一条 |
| ops.go:411 | `登录似乎没完成：官方 CLI 没写出令牌` | ✔ | ✘ | △ 缺"再试一次 / 看下面日志" |
| ops.go:449 | `grok 账号 %s 已有完整官方快照；要替换请用「导入会话文件」` | ✔ | ✔ | ✔ 但"快照"是黑话 |
| ops.go:453 | `没找到 grok CLI，先安装：curl -fsSL https://x.ai/cli/install.sh \| bash` | ✔ | ✔ | ✔ 齐全，但 `curl \| bash` 不该出现在错误行（不可复制、有安全暗示） |
| ops.go:473 | `设备码会话缺官方资料字段，写回本机 grok 会被拒绝；请用「导入会话文件」快照 ~/.grok/auth.json` | ✔ | ✔ | ✔ 内容齐全，黑话多 |
| ops.go:481 | `没找到 codex CLI，先安装官方 Codex CLI` | ✔ | △（说了"安装"没给方法） | △ 与另外三家不一致 |
| ops.go:511 | `没找到 kimi CLI；可以用「粘贴 API key」或「导入浏览器 cookie」` | ✔ | ✔ | ✔ 直接引用界面菜单名，示范级 |
| ops.go:564 | `kimi 这个号只有 key/cookie，没有会话文件可写回官方 kimi-code.json` | ✔ | ✘ | △ |
| ops.go:566 | `codex 不写官方 ~/.codex（避免 refresh 把官方会话作废）` | ✔ | ✘ | △ 设计性拒绝走错误通道，且没有可做的下一步 |
| ops.go:568 | `%s 没有本机官方客户端可切` | ✔ | ✘ | △ |
| ops.go:606 | `cursor 账号 %q 还没登录` | ✔ | ✘ | △ 缺"请先登录" |
| ops.go:609 | `cursor 账号 %q 的令牌已过期，重新登录` | ✔ | ✔ | ✔ |
| ops.go:681 | `grok 账号 %q 还没登录` | ✔ | ✘ | △ |
| ops.go:684 | `grok 账号 %q 不是完整官方会话，写回会逼 grok 重新网页登录；请先导入 ~/.grok/auth.json` | ✔ | ✔ | ✔ 超 20 字 |
| ops.go:717 | `kimi 这个号只有 key/cookie，没有会话文件可写回官方 kimi-code.json` | ✔ | ✘ | △ 与 564 完全重复 |
| ops.go:724 | `kimi 账号 %q 的会话文件不在 %s` | ✔ | ✘ | △ 且吐司里带真实绝对路径 |
| ops.go:731 | `kimi 账号 %q 不是完整 CLI 会话` | △ | ✘ | △ |
| ops.go:750 | `这个号就绑在官方会话文件上，先快照一份再切` | ✔ | ✔ | ✔ |
| ops.go:856 | `拒绝删除 %s：不在 leeway 会话目录里` | ✔ | ✘ | △ 内部保护，用户无事可做；带真实路径 |
| app.go:306 | `failureText("探测失败", ferr)` | △（ferr 无汉字时原因全丢） | ✘ | △ |
| app.go:656 | `failureText("切换失败", err.Error())` | ✘（英文详情被 `_` 丢弃） | ✘ | ✘ 最差一类 |
| app.go:682 | `failureText("退出失败", err.Error())` | ✘ | ✘ | ✘ |
| app.go:696 | `failureText("设置保存失败", err.Error())` | ✘ | ✘ | ✘ |
| app.go:711 | `failureText("清除缓存失败", err.Error())` | ✘ | ✘ | ✘ |
| app.go:724 | `failureText("保存失败", err.Error())` | ✘ | ✘ | ✘ |

**结构性问题（读代码得到，比单条更重要）**

1. **弹窗错误行不过滤库错误**：`promptview.go:57-59` 是 `errorLine(c, p, a.prompt.err, "")`——detail 传空，`failureText` 完全没参与。所以 `ops.go` 里 `os.ReadFile`、`writeFileAtomic`、`config.Load` 等返回的**英文库错误（常含真实绝对路径）会原样红字显示**在弹窗里，正是 `view.go:441-445` 注释声称不做的事。
2. **吐司丢弃详情**：五处 `a.toast, _ = failureText(...)` 把第二个返回值扔掉。含汉字的错误会拼进吐司，不含汉字的（库错误）只剩"切换失败"四个字，连 tooltip 都没有。
3. **同一动作两套动词**：删除类操作在标题里是"退出/删除"，在正文里是"删除"，在吐司里是"已清除/已移除/已删掉"，在按钮上是"退出登录/删除 API key"。
4. **裸 provider id 与显示名混用**：弹窗副标题用 `providerName()`（`Kimi Code`），确认框标题和所有吐司用裸 id（`kimi`、`opencode-go`）。
5. **确认框主按钮一律 critical 红**：`promptview.go:271-273` 对所有 `kind` 上红底，包括可逆的"切换"和"清除"。

---

## 三、确认框表

结构：`confirmSheet`（`promptview.go:255-285`）＝ 标题（`chrome`，**单行，无省略号**）+ 正文（`muted`，可换行）+ 次按钮 `取消` + 主按钮（红底，文字来自 `confirm.label`）。四个确认框全部共用同一个"取消"。

| # | file:line | 标题 | 正文 | 主按钮 | 次按钮 | 用户能否一眼看出点下去会发生什么 | 可逆？ |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | prompt.go:267-269 | `让本机 %s 改用 %s？` | `leeway 会把该账号的会话写给本机官方客户端；本机当前登录会被快照成 leeway 账号。` | `切换`（prompt.go:269） | `取消` | **一半**：说了"会切本机"和"会存一份旧登录"，但没说"旧登录若不在 leeway 里就永久丢"（该警告只在成功后的吐司 `app.go:664` 出现）；"写给""快照"是黑话；按钮"切换"没说切什么 | 是（有快照时）/ **否**（`Unsaved` 时旧登录找不回，且确认阶段不告知） |
| 2 | prompt.go:280-281 | `退出 %s 账号 %s？` | `会删除该账号在 leeway 里保存的私有会话、cookie 与配置里的 key，不影响本机官方客户端里的登录（cursor/grok 还会清掉私有 HOME）。` | `退出登录`（view.go:550） | `取消` | **一半**：正文说了删什么，但前后半句自相矛盾（"不影响本机官方客户端" vs "还会清掉私有 HOME"）；标题说"退出"、正文说"删除"；没说删完要重新登录 | **否**（登录被删，需重登） |
| 3 | prompt.go:283-284 | `删除 %s 账号 %s 的 API key？` | `只删掉 leeway 里存的这个 key，本机其它地方不受影响。` | `删除 API key`（view.go:552） | `取消` | **是**：范围说清了。缺"删了要重新粘贴" | **否**（key 需重新粘贴，且原值不可见无法恢复） |
| 4 | prompt.go:313-315 | `清除 cookie 缓存？` | `下一次探测会重新从浏览器读一次 cookie。config.json 里的密钥和账号不受影响。` | `清除` | `取消` | **是**：四条里写得最清楚 | **是**（下次探测自动重读） |

**确认框层面的问题**

- 四个框的次按钮都是"取消"，看不出"不做什么"；主按钮"切换/清除"也没带宾语。
- 主按钮全部红底（`promptview.go:271-273`），把"切换"（通常可逆）和"删除 key"（不可逆）画成同一危险等级——违反对比原则里"颜色要表示同一件事"。
- 确认框 1、2 的**不可逆分支只在事后吐司里说**：这是本块唯一"用户可能丢东西却事前没被告知"的地方，优先级最高。
- 标题用裸 provider id 且走 `chrome()` 单行渲染，最长的一条（`删除 opencode-go 账号 work 的 API key？`）接近宽度上限，再长的账号名/邮箱会被裁掉且没有省略号。
- 句式不统一：1 是"让本机 X 改用 Y？"，2 是"退出 X 账号 Y？"，3 是"删除 X 账号 Y 的 API key？"，4 是"清除 cookie 缓存？"——前三条是"操作 + 对象 + ？"，第四条没有对象。

---

## 四、不确定清单

1. **6 条 `prompt.title`（prompt.go:72/83/97/105/127/136）当前不显示**——依据是 `promptview.go:101-109`：`provider != ""` 时副标题走 `providerName()`。我判断为死文案，但没有测试断言过它们不显示，也没有其它读取点（已 grep `prompt.title`，只有 prompt.go 赋值和 promptview.go:103 一处读取）。**不确定**是否保留：如果将来 provider 为空也可能打开弹窗，它们会突然出现。
2. **不可达分支**：`promptview.go:97`（default 标题）、`prompt.go:256`（`已更新凭据` 兜底）、`ops.go:163`（缓存还新）、`ops.go:323`、`ops.go:380`、`ops.go:568`。依据分别是 `active()` 判定、三处调用都传非空、GUI 唯一调用点传 `force=true`、`openSessionFile`/`openCLI` 的出现条件（`view.go:586-604`）。**不确定**是否有非 GUI 调用方（`internal/gui` 之外没有，已 grep；但测试 `gui_test.go` 直接调用了这些函数，将来也可能有新调用方），所以我按"保留但标注"处理，没有建议删除。
3. **`default` 是否要中文化（ops.go:27）**——它同时出现在确认框标题、吐司、账号列表里，可能被别的范围当作标识符处理。**不确定**，需要全局口径（`docs/copy-audit/glossary-draft.md` 若有结论应以此为准）。
4. **`app.go:327` 的 `正常/失败/未配置` 是否算"用户可见"**——它每次渲染标题栏都算一次，同时被朗读。我按"可见"计入。**不确定** `overview()` 把"有凭据未探测"算进 `正常` 是刻意的产品口径（注释说这是为了不再出现"已找到凭据"配"未配置"表头），还是可以改词。**建议只改词（正常→可用），不动计数逻辑**。
5. **`app.go:588` 的到期格式 `01-02 15:04`**：跨年时看不出是哪一年。**不确定**是否有意（省宽度）。
6. **`ops.go:453` 的 `curl -fsSL … | bash` 是否允许出现在界面里**——我倾向改成可复制的独立行或文档链接，但"功能含义不变"要求它仍要告诉用户怎么装；最终形态取决于项目是否愿意在 GUI 里展示 shell 命令。**不确定**。
7. **确认框主按钮的红底是否算文案问题**——它是视觉问题，但影响"点下去会发生什么"的判断，我放在确认框一节而非表格。**不确定**是否属于本次文案范围。
8. **`view.go:550/552` 的 `退出登录`/`删除 API key`** 严格说不在我的 5 个文件里，但它们是确认框主按钮文字、`prompt.go:290` 直接引用。我把它列进确认框表并标了出处，**不算进本块 122 条**。
9. **`settingsview.go` 里的 `已复制 JSON`/`已复制路径`/`开机自启设置失败`** 也在吐司通道里，但不属于我这一块，未计入。`trend.go:284` 的 `burnToast` 同理（它有自己的 `c.Announce`，见 `trend.go:296-301`）。

---

## 五、这一块的文案问题（一段话）

这一块的文案底子比想象中好——`ops.go:511`（"没找到 kimi CLI；可以用「粘贴 API key」或「导入浏览器 cookie」"）和 `ops.go:387` 已经做到了"原因 + 下一步"并引用界面里的真实菜单名，但整体问题是**三件事没有统一口径**：第一，**失败的表达被加工层吃掉了**，`view.go` 明明有"英文库错误不上屏、只进 tooltip"的规则，弹窗错误行却整条绕过它（`promptview.go:58` 传空 detail），五处吐司又把详情返回值直接丢弃（`app.go:656/682/696/711/724`），于是用户最需要原因的时候只剩"切换失败""保存失败"；第二，**同一件事有三套说法**——删登录在标题叫"退出"、正文叫"删除"、吐司叫"已清除/已移除/已删掉"，provider 一会儿是显示名一会儿是裸 id，`默认`账号还写成 `default`，四个确认框的句式也各写各的；第三，**不可逆操作事前不交底**——切本机时"旧登录找不回"只在成功吐司里说（`app.go:664`），删登录的正文一边保证"不影响本机"一边又说"会清掉私有 HOME"，而四个确认框的主按钮一律涂成危险红、次按钮一律叫"取消"，颜色和文字都没有承担"点下去会发生什么"的责任。此外还有一类隐性问题：真实绝对路径会随错误吐司和确认框进到界面（`ops.go:724/856`、`app.go:667`），截图或录屏就会带出用户名，这与项目"界面只显示凭据来源、不显示内容"的硬约束同源，建议一并收紧。
