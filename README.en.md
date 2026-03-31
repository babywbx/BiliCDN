<div align="center"><a name="readme-top"></a>

# 🎬 BiliCDN Web

Interactive node browser for BiliCDN.

[简体中文](./README.md) · **English** · [Live Demo](https://bilicdn.pages.dev)

[![][github-license-shield]][github-license-link]

</div>

## 📖 About

This is the web frontend for [BiliCDN](https://github.com/babywbx/BiliCDN), providing an interactive CDN node browsing interface. The page is pure static HTML that fetches data at runtime from the [`data`](https://github.com/babywbx/BiliCDN/tree/data) branch — no build step required.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ✨ Features

- **Interactive table** — Search, sort, filter, and paginate all nodes
- **Click to copy** — Click any domain to copy to clipboard
- **Region filtering** — Filter by geographic area (Municipalities, East, South...)
- **Multi-format download** — "Download" button in the header for JSON / YAML / Text / Markdown / Raw
- **API endpoints** — "API" button to copy CDN-accelerated data endpoint URLs
- **Stats overview** — Domain count, region count, type breakdown with hover tooltips
- **Live data** — Automatically fetches latest data on every visit, no redeployment needed
- **Dark theme** — Modern dark UI, fully responsive
- **Lighthouse perfect score** — Accessibility 100 / Best Practices 100 / SEO 100

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🔗 API Endpoints

When deployed on Cloudflare Pages, all data files are served via CDN for fast global access:

```
https://bilicdn.pages.dev/nodes.json
https://bilicdn.pages.dev/nodes.yml
https://bilicdn.pages.dev/nodes.txt
https://bilicdn.pages.dev/nodes.md
https://bilicdn.pages.dev/domains.txt
```

Powered by Cloudflare Pages Functions that reverse-proxy GitHub data with 6-hour edge caching. No direct GitHub access needed from China.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ⚙️ Architecture

```
User → bilicdn.pages.dev (Cloudflare CDN)
         ├── /              → index.html (static page)
         ├── /nodes.json    → CF Functions → GitHub raw (proxy + cache)
         ├── /domains.txt   → CF Functions → GitHub raw (proxy + cache)
         └── /api/updated   → CF Functions → GitHub API (proxy + cache)
```

- **Zero build** — Single `index.html` + CF Functions, no framework, no dependencies
- **Zero maintenance** — Template never needs updating when data changes
- **Global acceleration** — Cloudflare edge caching, China-friendly
- **Deploy to** — Cloudflare Pages (recommended), GitHub Pages, Vercel

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🚀 Deployment

### Cloudflare Pages

1. Connect GitHub repository
2. Branch: `web`
3. Build command: (empty), Output directory: `/`

### GitHub Pages

1. Settings → Pages → Source: Deploy from a branch
2. Branch: `web` / `/ (root)`

### Local Preview

```bash
# 1. Switch to web branch
git checkout web

# 2. Generate local test data (requires bilicdn binary from main branch)
git checkout main -- bilicdn 2>/dev/null || (git stash && git checkout main && go build -o bilicdn . && git checkout web && git stash pop)
mkdir -p data
./bilicdn -be 1 -se 5 -gotcha=false -quiet -o data/domains.txt
./bilicdn convert -i data/domains.txt -o data/nodes.json
./bilicdn convert -i data/domains.txt -o data/nodes.yml
./bilicdn convert -i data/domains.txt -o data/nodes.txt
./bilicdn convert -i data/domains.txt -o data/nodes.md

# 3. Temporarily point to local data
sed -i '' "s|https://raw.githubusercontent.com/babywbx/BiliCDN/data|data|" index.html

# 4. Start local server
python3 -m http.server 8080
# Open http://localhost:8080

# 5. Restore after testing
git checkout -- index.html
rm -rf data/ bilicdn
```

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 📄 License

This project is licensed under the MIT License. See the [`LICENSE`](https://github.com/babywbx/BiliCDN/blob/main/LICENSE) file for details.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

Copyright © 2026 Babywbx.

<!-- LINK GROUP -->

[github-license-shield]: https://img.shields.io/github/license/babywbx/BiliCDN?style=flat-square&logo=opensourceinitiative&labelColor=black&color=white
[github-license-link]: https://github.com/babywbx/BiliCDN/blob/main/LICENSE
[back-to-top]: https://img.shields.io/badge/-BACK_TO_TOP-151515?style=flat-square
