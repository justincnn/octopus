<div align="center">

# 🐙 Octopus — 轻量 LLM API 统一网关

**把多个大模型渠道收编成一个 OpenAI 兼容接口，并让每个渠道变得可运维、可控成本、可上生产。**

[![License](https://img.shields.io/badge/license-MIT-green)](#License)
[![Python](https://img.shields.io/badge/backend-Go-blue)]()
[![Frontend](https://img.shields.io/badge/frontend-React%20%2B%20Vite-blue)]()

本仓库是对 [`bestruirui/octopus`](https://github.com/bestruirui/octopus) 的**深度增强 fork**，在保持原版「单一二进制、零外部依赖」轻量架构的基础上，加入了大量面向真实生产与大模型聊天场景的自用增强。

</div>

---

## 它解决什么问题

当你有多个大模型上游（OpenAI、Claude、Gemini、腾讯混元、Mistral、自建 vLLM……）时，Octopus 能：

- **统一入口**：只暴露一个 OpenAI 兼容 API，业务方不用关心背后是哪个渠道。
- **多格式互通**：OpenAI / OpenAI-Responses / Anthropic / Gemini 格式请求自动转换。
- **智能路由**：做负载均衡、故障转移、熔断，一个渠道挂掉自动切下一个。
- **Key 池管理**：一个渠道配多个 Key，失败自动换 Key，不用再手写重试。
- **代理池**：每个渠道可配置多个出口 IP，应对 IP 限流/封禁。
- **生产级仪表盘**：内置 Web 管理后台，可视化查看渠道健康、请求日志、成本。

原版是「能用」的网关，本 fork 的重点是把那些**只有真实部署后才需要**的能力补上：Key 自动轮换、模型可用性测试、未匹配模型映射、代理池、紧凑运维界面等。

---

## ✨ 相比原版的核心增强

> 以下均为本 fork 在你可 fork 到真实生产场景中反复踩坑后落地的改动，完整提交记录见 [git log](#git-历史) 。

### 1. Key 池全生命周期管理
- 渠道支持**多 Key**（批量粘贴，空格/逗号/换行分隔）。
- 内置 Key 状态机：`active`（可用） / `invalid`（失效），并记录**失效原因、失败次数、时间**。
- 可**删除失效 Key**，也可一键禁用/启用/恢复。
- 管理弹窗 **10 秒自动刷新**，实时看到 Key 健康度。

### 2. 每渠道 key 轮询策略可独立配置
| 策略 | 行为 |
| --- | --- |
| `priority`（默认） | 主 Key 优先使用，失败后随机切换备用 Key |
| `round_robin` | 轮流使用（轮询） |
| `random` | 随机挑选 |
| `least_used` | 选当前被用得最少的 Key |

每个渠道用 `key_strategy` 字段单独指定，Key 池弹窗里即可切换。

### 3. 模型可用性测试（渠道配置一条龙）
- **流式测试模型**：后端发起 SSE 流式请求，收到首帧即判定可用，不等整段生成——测试速度快到一个量级。
- **一键加入可用模型**：测试通过的模型，点一下批量写回渠道；已选模型可**覆盖式**更新（已选 = 测试通过后的最新集）。
- **模型候选弹窗**：拉取上游模型后不再默认乱勾，弹出多选框，支持**全选 / 反选 / 全不选**并显示已选数量，确认后才写入。
- **细节修复**：修掉了测试模型时的 412（Go 构造请求未设 `ContentLength` 走 chunked）、401（key 被打码后直传上游）等真实坑。
- OpenAI 计费字段自动用 `max_completion_tokens`（适配新 API）。

### 4. 未匹配模型 → 优雅映射
- **模糊匹配**：上游返回一个没登记的模型名时，按名称自动模糊匹配到已有模型。
- **一键批量匹配 + 别名自动同步**：全量候选只读地在界面上展示，逐条确认后写入别名表，别名跟随模型目录自动同步。
- **关键词搜索**：整个模型目录支持按关键词检索，方便人工映射。

### 5. 分组管理（未分组模型不再裸奔）
- 新增 `/model/ungrouped` 接口，精确统计**真正未分组的模型数**，工具条上显示徽章。
- 未分组弹窗改为**分组管理**视图——支持一键 / 自动把模型加入分组。
- 分组可用**模型启用开关** + 独立的轮询机制（对齐渠道 key 池）。

### 6. 代理池 + 粘滞出口
- 每个渠道可配 **`ProxyPool`（推池，多出口 IP，每请求轮换）** 和 **`Sticky`（粘滞开关）**。
- 出口优先级：`proxy_pool` > `channel_proxy` > 系统代理。
- `sticky=true` 时按渠道 ID 哈希固定在同一个出口；否则全局轮换。专治上游按 IP 限流。

### 7. embedding / rerank 中转
- 直接透传 embedding 与 rerank 模型调用，让向量检索类任务也能走同一个网关。

### 8. Mistral Conversations 适配器
- 内置 `internal/relay/mistral_conversation.go`，可把 **Mistral `/conversations`** 接口（如 glm-5-2）无感接入 OpenAI 客户端，格式自动互转（messages↔inputs、outputs→choices、SSE 流逐块映射）。

### 9. 运维面板「紧凑模式」
- 所有列表页**默认收起成列表视图**，渠道列表 / 日志卡片做成**紧凑单行**——一屏看 3 倍数据。
- 折线图 Y 轴归一化，容量优先级不因某个渠道抖动而对比。
- 修复分组列表下滚失效（改用可滚容器）。
- 修复 Service Worker 缓存导致改版后白屏/无样式（assets 网络直取 + 缓存版本管理）。

### 10. 精简与去重
- **移除版本更新检查模块**（自用无需联网比对版本）。
- 定价拉取：直连失败自动走代理重试，并过滤自研/杂项模型名。

---

## 🧠 架构与设计取舍

- **单二进制发布**：前端（Vite + React）构建产物直接 `embed` 进 Go 二进制，部署只有一个可执行文件，1 GB 内存的小 VPS 也跑得动，无独立 Node 进程。
- **存储**：默认 SQLite 单文件（`data/data.db`），换 Postgres/MySQL 只需改环境变量。
- **纯运维决策**：本地**不预处理请求**（不对上下文做 token 估算/截断），请求原样透传给上游、由上游自己校验——避免「本地核算 1 token≈4字符」严重误估导致真实超长上下文被漏判。超大请求请调用侧控制上下文。

---

## 🚀 快速开始（Docker Compose）

前置：已装 Docker + Docker Compose。

```bash
git clone https://github.com/justincnn/octopus.git
cd octopus

# 修改 docker-compose.yml，把 8081 换成你想对外暴露的端口
docker compose up -d

# 浏览器打开
# http://localhost:8081
```

首次打开会引导你创建管理员账号。

> 说明：`docker-compose.yml` 默认把宿主机 `./data` 挂为数据卷，配好的渠道、模型、Key 都会持久化，重建容器不丢。

### 手动二进制运行

```bash
# 导出配置（也可直接用环境变量）
export OCTOPUS_SERVER_PORT=8080
export OCTOPUS_DATABASE_TYPE=sqlite
export OCTOPUS_DATABASE_PATH=data/data.db

./octopus-bin
```

## ⚙️ 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `OCTOPUS_SERVER_PORT` | `8080` | HTTP 监听端口 |
| `OCTOPUS_DATABASE_TYPE` | `sqlite` | `sqlite` / `mysql` / `postgres` |
| `OCTOPUS_DATABASE_PATH` | `data/data.db` | SQLite 文件路径 |
| `OCTOPUS_DATABASE_DSN` | - | 非 SQLite 时填连接串 |

## 🧰 管理后台

| 页面 | 功能 |
| --- | --- |
| 统计 | 调用量、Token、成本趋势，渠道健康 |
| 渠道 | 渠道/Key/代理池/轮询策略的增删改，模型可用性测试 |
| 模型 | 模型目录、未匹配映射、别名管理、分组 |
| 日志 | 单个请求级日志（紧凑列表，几秒刷新） |

---

## 🛠️ 本地开发

### 前端

```bash
cd web
pnpm install
pnpm dev        # 开发模式，热更新
```

> 构建产物会写到 `static/out`（Go embed 读取），改完前端后需重新构建。

### 后端

```bash
go run ./cmd/server     # 或 go build -o octopus-bin main.go
```

### 一键构建发布包

```bash
# 1. 构建前端（产物落到 static/out）
cd web && pnpm install && pnpm build && cd ..

# 2. 构建带前端的单二进制
GOOS=linux GOARCH=amd64 go build -o octopus-bin main.go   # 换 arch 同理
```

## ❓ FAQ / 常见问题

**Q: 单条消息超长，请求会不会被本地截断？**
不会。本 fork 明确去掉本地上下文估算与截断（曾因按「1 token ≈ 4 个非中文字符」粗估，严重低估英文/代码/JSON，导致超长请求漏判）。请把 `max_context_tokens` 交给上游渠道去校验，或在调用侧控制上下文。

**Q: 一个渠道配了多个 Key，坏了一个会怎样？**
Key 池会打上 `invalid` 标记，路由自动避开它并切到健康 Key（按你选的策略）。无效 Key 可在后台看到原因、失败次数，并手动删除。

**Q: 某上游对 IP 限流，怎么换出口？**
在渠道上填 `proxy_pool`（多出口 IP），需要稳定出口时开着 `sticky`，请求就固定走指定 IP。

**Q: 我拉到的模型列表很乱，怎么归口？**
后台「未匹配模型」可以把它们一键批量匹配到已有模型，或直接加入某个分组；也可以建立名称别名。

**Q: 前端改了没生效？**
前端产物必须经 `vite build` 落到 `static/out`，再 `go build` 进二进制才有用；线上替换后建议浏览器 `Ctrl+Shift+R` 强刷一次。

---

## 🗺️ 目录结构（节选）

```
octopus/
├── main.go                 # 入口
├── cmd/server/             # 服务端启动
├── internal/
│   ├── relay/              # 请求路由/格式互转/熔断/代理
│   │   ├── mistral_conversation.go
│   │   └── ...
│   ├── op/                 # 渠道/Key/模型 管理操作（含 key 状态机、轮询）
│   ├── model/              # GORM 数据模型
│   ├── price/             # 模型定价
│   └── server/            # HTTP/管理 API
├── web/                    # Vite + React 前端
└── static/out/            # 前端构建产物（embed 进二进制）
```

## 📜 License

本仓库基于 [MIT](LICENSE) 发布。原版版权归 [bestruirui/octopus](https://github.com/bestruirui/octopus) 所有。

---

> 本项目仅供学习、研究、自用与技术交流使用。