<div align="center"><a name="readme-top"></a>

# 🎬 BiliCDN Web

BiliCDN 的交互式节点浏览页面。

**简体中文** · [English](./README.en.md) · [在线浏览](https://bilicdn.pages.dev)

[![][github-license-shield]][github-license-link]

</div>

## 📖 简介

这是 [BiliCDN](https://github.com/babywbx/BiliCDN) 项目的 Web 前端，提供交互式的 CDN 节点浏览界面。页面为纯静态 HTML，数据在运行时从 [`data`](https://github.com/babywbx/BiliCDN/tree/data) 分支实时获取，无需构建步骤。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ✨ 功能

- **交互式表格** — 搜索、排序、筛选、分页
- **点击复制** — 点击域名即可复制到剪贴板
- **区域筛选** — 按地理大区快速过滤（直辖市、华东、华南...）
- **多格式下载** — JSON / YAML / Text / Markdown / Raw
- **统计概览** — 域名总数、区域数、类型分布，悬停查看中文释义
- **实时数据** — 每次访问从 GitHub 获取最新数据，无需重新部署
- **深色主题** — 现代暗色 UI，响应式适配

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ⚙️ 架构

```
web 分支 (本分支)           data 分支
  index.html     ──fetch──→  nodes.json     (交互式表格数据)
  (纯静态模板)               domains.txt    (下载链接)
                             nodes.yml/txt/md
```

- **零构建** — 单个 `index.html`，无框架、无依赖
- **零维护** — 数据更新时无需修改模板
- **可部署到** — Cloudflare Pages / Workers、GitHub Pages、Vercel 等任何静态托管

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🚀 部署

### Cloudflare Pages

1. 连接 GitHub 仓库
2. 分支选择 `web`
3. 构建命令留空，输出目录填 `/`

### GitHub Pages

1. Settings → Pages → Source: Deploy from a branch
2. Branch: `web` / `/ (root)`

### 本地预览

```bash
# 1. 切到 web 分支
git checkout web

# 2. 生成本地测试数据（需要先在 main 分支编译 bilicdn）
git checkout main -- bilicdn 2>/dev/null || (git stash && git checkout main && go build -o bilicdn . && git checkout web && git stash pop)
mkdir -p data
./bilicdn -be 1 -se 5 -gotcha=false -quiet -o data/domains.txt
./bilicdn convert -i data/domains.txt -o data/nodes.json
./bilicdn convert -i data/domains.txt -o data/nodes.yml
./bilicdn convert -i data/domains.txt -o data/nodes.txt
./bilicdn convert -i data/domains.txt -o data/nodes.md

# 3. 临时修改数据源为本地
sed -i '' "s|https://raw.githubusercontent.com/babywbx/BiliCDN/data|data|" index.html

# 4. 启动本地服务器
python3 -m http.server 8080
# 打开 http://localhost:8080

# 5. 测试完毕后还原
git checkout -- index.html
rm -rf data/ bilicdn
```

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 📄 许可证

本项目采用 MIT 许可证。详情请见 [`LICENSE`](https://github.com/babywbx/BiliCDN/blob/main/LICENSE) 文件。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

Copyright © 2026 Babywbx.

<!-- LINK GROUP -->

[github-license-shield]: https://img.shields.io/github/license/babywbx/BiliCDN?style=flat-square&label=%E8%AE%B8%E5%8F%AF%E8%AF%81&logo=opensourceinitiative&labelColor=black&color=white
[github-license-link]: https://github.com/babywbx/BiliCDN/blob/main/LICENSE
[back-to-top]: https://img.shields.io/badge/-BACK_TO_TOP-151515?style=flat-square
