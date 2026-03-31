<div align="center"><a name="readme-top"></a>

# 🎬 BiliCDN

一个高性能的哔哩哔哩 CDN 节点发现与分类工具。

**简体中文** · [English](./README.en.md) · [在线浏览](https://bilicdn.pages.dev)

[![][ci-shield]][ci-link]
[![][update-data-shield]][update-data-link]
[![][last-updated-shield]][last-updated-link]
[![][github-license-shield]][github-license-link]

</div>

<details>
<summary><kbd>目录</kbd></summary>

- [📖 项目简介](#-项目简介)
- [✨ 功能特性](#-功能特性)
- [⚙️ 工作原理](#️-工作原理)
- [🚀 如何使用数据](#-如何使用数据)
- [🛠️ 本地开发](#️-本地开发)
  - [前置要求](#前置要求)
  - [运行扫描](#运行扫描)
  - [格式转换](#格式转换)
  - [命令行参数](#命令行参数)
- [🤝 参与贡献](#-参与贡献)
- [⚠️ 免责声明](#️-免责声明)
- [📄 许可证](#-许可证)

</details>

## 📖 项目简介

BiliCDN 是一个自动化工具，它通过已知的 B 站 CDN 域名命名规则生成候选域名，然后通过 DNS 解析和 HTTP 检测来验证节点是否存活，最终按地理区域和云厂商进行分类。生成的节点列表会自动提交到 `data` 分支，提供多种格式供使用。

本项目旨在为开发者和网络研究者提供持续更新、可靠的 B 站 CDN 基础设施目录。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ✨ 功能特性

- **🤖 完全自动化**：通过 GitHub Actions 每周自动更新，支持手动触发。
- **⚡️ 高性能**：两级流水线架构（DNS 工作池 + HTTP 工作池），速度可达 1900+ 域名/秒。
- **🧠 DNS 自动调优**：自动测试并选择当前网络环境下最优的 DNS 配置。
- **🪶 零外部依赖**：纯 Go 标准库，仅依赖 `golang.org/x/term`。
- **🌍 全面覆盖**：315+ 城市级地区、15+ 运营商/云厂商、UPOS 存储节点、Gotcha CDN 节点及 Akamai 等外部 CDN。
- **📝 多格式输出**：生成 JSON、YAML、TXT、Markdown，按地理区域自动分组。
- **🗺️ 智能分类**：自动识别省份、城市、云厂商和 CDN 类型。
- **🔄 原子写入**：使用文件锁和原子替换，防止数据损坏。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ⚙️ 工作原理

1. **自动调优**：探测 DNS 服务器，基准测试不同配置，选择最快的方案。
2. **域名生成**：根据 B 站 CDN 已知命名规则，生成约 450 万个候选域名。
3. **DNS 解析**：1500 个并发 DNS 工作协程验证域名是否存在（过滤 NXDOMAIN）。
4. **HTTP 验证**：50 个 HTTP 工作协程通过 HEAD 请求确认节点存活。
5. **输出**：结果排序去重后原子写入，`bilicdn convert` 生成按区域分组的多种格式。
6. **发布**：`wbxBot` 每周自动将更新数据提交到 `data` 分支。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🚀 如何使用数据

### 在线浏览

通过 [**bilicdn.pages.dev**](https://bilicdn.pages.dev) 在线查看所有 CDN 节点：

- 交互式表格，支持搜索、排序、按区域筛选
- 点击域名即可复制
- 通过页面顶部「下载」按钮直接下载各格式文件
- 通过「API」按钮一键复制数据接口链接

### API 引用

所有数据文件可通过 Cloudflare CDN 加速访问，适合程序直接引用：

```
https://bilicdn.pages.dev/nodes.json
https://bilicdn.pages.dev/nodes.yml
https://bilicdn.pages.dev/nodes.txt
https://bilicdn.pages.dev/nodes.md
https://bilicdn.pages.dev/domains.txt
```

### 直接下载

也可以从 [`data` 分支][data-branch-link]下载原始数据文件：

| 文件 | 说明 |
| --- | --- |
| `domains.txt` | 纯域名列表，每行一个（扫描器原始输出） |
| `nodes.json` | JSON 格式，按区域分组 |
| `nodes.yml` | YAML 格式，按区域分组 |
| `nodes.txt` | 纯文本格式，按区域分组 |
| `nodes.md` | Markdown 格式，按地理大区分段 |

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🛠️ 本地开发

### 前置要求

- Go 1.26+

### 运行扫描

```bash
# 编译
go build -o bilicdn .

# 使用默认设置运行（自动调优 DNS，全量扫描）
./bilicdn

# 快速扫描（缩小范围）
./bilicdn -be 3 -se 10 -gotcha=false

# 指定 DNS 策略
./bilicdn -dns 2    # 仅国内 DNS
./bilicdn -dns 3    # 系统 DNS

# 自定义输出路径
./bilicdn -o /tmp/results.txt

# CI 模式（无 TUI 进度条）
./bilicdn -quiet

# 中断后从断点继续（全量扫描约 76 分钟，支持随时中断续扫）
./bilicdn -resume
```

### 格式转换

```bash
# 默认：domains.txt → nodes.json
./bilicdn convert

# 指定输入输出
./bilicdn convert -i data/domains.txt -o data/nodes.json
./bilicdn convert -i data/domains.txt -o data/nodes.yml
./bilicdn convert -i data/domains.txt -o data/nodes.txt
./bilicdn convert -i data/domains.txt -o data/nodes.md

# 强制指定格式（忽略扩展名）
./bilicdn convert -o output -f yaml
```

### 命令行参数

**扫描器：**

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-dns` | `0` | DNS 策略：0=自动, 1=全球, 2=国内, 3=系统 |
| `-c` | `0` | 并发工作协程数（0 = 自动） |
| `-d` | `bilivideo.com` | 目标域名 |
| `-o` | `data/domains.txt` | 输出文件路径 |
| `-bs` / `-be` | `1` / `10` | 机房编号范围 |
| `-ss` / `-se` | `1` / `50` | 服务器编号范围 |
| `-gotcha` | `true` | 启用 Gotcha 模式扫描 |
| `-quiet` | `false` | 日志模式（无 TUI） |
| `-debug` | `false` | 输出错误日志到 scanner_errors.log |
| `-resume` | `false` | 从上次断点继续扫描 |

**格式转换：**

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-i` | `data/domains.txt` | 输入域名文件 |
| `-o` | `data/nodes.json` | 输出文件（.json/.yml/.txt/.md） |
| `-f` | （自动检测） | 强制格式（json/yaml/txt/md） |

<div align="right">

[![][back-to-top]](#readme-top)

</div>


## 🤝 参与贡献

欢迎任何形式的贡献！你可以随时提交 Issue 或 Pull Request。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ⚠️ 免责声明

本项目仅供学习研究和网络技术探索之用。

- 本项目与哔哩哔哩（bilibili.com）**无任何关联**，非官方项目，未获得 bilibili 的授权或认可。
- 本项目**不涉及**任何形式的破解、逆向工程、渗透测试、入侵或绕过安全机制等行为。
- 本项目仅通过公开的 DNS 解析和标准 HTTP 请求发现公网可达的 CDN 域名，**不访问、不获取、不存储**任何受保护的内容或用户数据。
- 使用者应遵守所在地区的法律法规及 bilibili 的服务条款，**使用本项目所产生的一切后果由使用者自行承担**。
- 项目作者**不对因使用本项目而产生的任何直接或间接损失负责**，包括但不限于服务中断、数据丢失、法律纠纷等。
- 如果本项目的存在或使用侵犯了任何第三方的合法权益，请通过 Issue 联系，我们将及时处理。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 📄 许可证

本项目采用 MIT 许可证。详情请见 `LICENSE` 文件。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

Copyright © 2026 Babywbx.

<!-- LINK GROUP -->

[ci-shield]: https://img.shields.io/github/actions/workflow/status/babywbx/BiliCDN/ci.yml?label=CI&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[ci-link]: https://github.com/babywbx/BiliCDN/actions/workflows/ci.yml
[update-data-shield]: https://img.shields.io/github/actions/workflow/status/babywbx/BiliCDN/update-data.yml?label=%E8%87%AA%E5%8A%A8%E6%9B%B4%E6%96%B0%20CDN%20%E6%95%B0%E6%8D%AE&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[update-data-link]: https://github.com/babywbx/BiliCDN/actions/workflows/update-data.yml
[last-updated-shield]: https://img.shields.io/github/last-commit/babywbx/BiliCDN/data?label=%E6%95%B0%E6%8D%AE%E6%9C%80%E5%90%8E%E6%9B%B4%E6%96%B0&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[last-updated-link]: https://github.com/babywbx/BiliCDN/tree/data
[github-license-shield]: https://img.shields.io/github/license/babywbx/BiliCDN?style=flat-square&label=%E8%AE%B8%E5%8F%AF%E8%AF%81&logo=opensourceinitiative&labelColor=black&color=white
[github-license-link]: https://github.com/babywbx/BiliCDN/blob/main/LICENSE
[back-to-top]: https://img.shields.io/badge/-BACK_TO_TOP-151515?style=flat-square
[data-branch-link]: https://github.com/babywbx/BiliCDN/tree/data
