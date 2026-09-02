# ACTX CLI

ACTX 是一个在本地运行的开源编程代理（coding agent）CLI，由 OpenAI Codex 的 Go 移植版改造而来。它可以执行命令、修改文件，并协助完成软件开发任务。支持通过 DeepSeek / OpenAI 等 Chat Completions 兼容 API 进行对话与工具调用。

## 功能特性

- **交互式对话 (TUI)**：基于 Bubble Tea 的终端界面，支持流式输出、审批请求、斜杠命令
- **非交互式执行**：`actx exec "prompt"` 单轮执行并输出结果
- **工具调用**：内置 `shell`（执行命令）、`apply_patch`（修改文件）、`web_search`（网络搜索）
- **多 Provider 配置**：通过 JSON 配置支持 DeepSeek、OpenAI 等多个模型提供商
- **会话管理**：`list` / `delete` / `resume` 列出、删除、恢复历史会话
- **App Server**：通过 stdio 提供 JSON-RPC 2.0 协议，可供桌面客户端集成
- **审批与沙箱策略**：`--approval-policy` 和 `--sandbox` 控制命令执行的权限与隔离
- **计划模式 / 子代理**：仅规划不执行、独立运行子任务
- **跨会话记忆**：`~/.actx/memory.json` 持久化记忆

## 环境要求

- Go 1.26+
- Windows / macOS / Linux

## 快速开始

### 1. 构建

```bash
make build          # 生成 actx.exe
# 或直接
go build -o actx.exe ./cmd/actx/
```

### 2. 配置 API Key

```bash
./actx.exe login --api-key sk-xxxx          # 使用 API Key
./actx.exe login                            # 交互式登录（可选 API Key 或 OAuth）
./actx.exe doctor                           # 检查认证、配置状态
```

### 3. 配置模型提供商

项目级配置放在 `.actx/config.json`（仓库根目录），用户级配置放在 `C:\Users\<用户名>\.actx\config.json`。两者冲突时以用户级配置为准。

```json
{
  "model_provider": "deepseek",
  "model_providers": {
    "deepseek": {
      "base_url": "https://api.deepseek.com/v1",
      "models": ["deepseek-v4-flash", "deepseek-v4-pro"]
    },
    "openai": {
      "base_url": "https://api.openai.com/v1",
      "models": ["gpt-4o", "o4-mini"]
    }
  }
}
```

- `model_provider`：选择当前使用的提供商
- `model_providers`：每个提供商独立的 `base_url` 和 `models` 列表（默认使用第一个模型）

### 4. 运行

```bash
./actx.exe                                  # 交互式 TUI 模式
./actx.exe exec "What is 2+2?"              # 非交互模式
./actx.exe -m deepseek-v4-pro exec "..."    # 指定模型
./actx.exe --sandbox read-only exec "..."   # 只读沙箱模式
./actx.exe --json exec "say hi"             # JSON 事件输出
```

## 命令

| 命令 | 说明 |
|------|------|
| `actx` | 启动交互式 TUI |
| `actx exec [prompt]` | 非交互式单轮执行 |
| `actx app-server` | 通过 stdio 运行 JSON-RPC App Server |
| `actx login` | 登录（API Key 或 OAuth） |
| `actx logout` | 登出并删除凭据 |
| `actx list` | 列出已保存的会话 |
| `actx delete <id>` | 删除指定会话 |
| `actx resume <id>` | 恢复指定会话 |
| `actx doctor` | 诊断认证与配置 |
| `actx --version` | 显示版本号 |

### 常用标志

| 标志 | 说明 |
|------|------|
| `-m, --model` | 指定模型 |
| `-p, --provider` | 指定模型提供商 |
| `-c, --config` | 指定配置文件路径 |
| `-o, --override key=value` | 覆盖配置项 |
| `--approval-policy` | `never` / `on-failure` / `on-request` / `on-untrusted` |
| `--sandbox` | `none` / `read-only` / `workspace-write` / `danger-full-access` |
| `--base-url` | 覆盖 API Base URL |
| `--json` | 以 JSON 行输出事件 |

## TUI 斜杠命令

| 命令 | 说明 |
|------|------|
| `/help` | 显示帮助 |
| `/clear` | 清空对话历史 |
| `/compact` | 压缩上下文 |
| `/model [name]` | 显示或切换模型 |
| `/init` | 生成 AGENTS.md 项目模板 |
| `/diff` | 显示 git diff |
| `/review` | 提交 AI 代码审查 |
| `/status` | 显示会话状态与上下文占用 |
| `/approvals [policy]` | 显示/切换审批策略 |
| `/new` | 开始新会话 |
| `/undo` | 回滚上一步 |
| `/logout` | 登出 |
| `/mcp` | 显示 MCP 服务器状态 |
| `/mention <path>` | 将文件内容加入上下文 |
| `/skills` | 浏览技能 |
| `/quit` | 退出 |

## 项目结构

```
cmd/actx/           入口 (main.go)
internal/
  agent/            代理核心: 会话、工具调用、子代理、记忆、网络搜索
  applypatch/       apply_patch 补丁解析与应用
  appserver/        JSON-RPC App Server (stdio)
  cli/              cobra 命令定义
  config/           配置加载、审批/沙箱策略、执行规则
  exec/             命令执行
  login/            API Key / OAuth 登录
  model/            Chat Completions 客户端、SSE 解析、工具定义
  protocol/         事件与消息类型
  rollout/          会话持久化
  sandbox/          沙箱类型与平台实现
  tools/            工具注册表、执行器、输出与错误
  tui/              Bubble Tea 终端界面
```

## 开发

```bash
make fmt          # 格式化代码
make vet          # 静态检查
make test         # 单元测试
make build        # 编译产物
```

## 许可

本项目基于 OpenAI Codex CLI 的 Go 移植，相关版权归原作者所有。
