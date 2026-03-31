<div align="center"><a name="readme-top"></a>

# 🎬 BiliCDN

A high-performance tool to discover and catalog Bilibili's CDN nodes across China and worldwide.

[简体中文](./README.md) · **English** · [Live Demo](https://bilicdn.pages.dev)

[![][ci-shield]][ci-link]
[![][update-data-shield]][update-data-link]
[![][last-updated-shield]][last-updated-link]
[![][github-license-shield]][github-license-link]

</div>

<details>
<summary><kbd>Table of Contents</kbd></summary>

- [📖 About The Project](#-about-the-project)
- [✨ Features](#-features)
- [⚙️ How It Works](#️-how-it-works)
- [🚀 How to Use the Data](#-how-to-use-the-data)
- [🛠️ Local Development](#️-local-development)
  - [Prerequisites](#prerequisites)
  - [Running the Scanner](#running-the-scanner)
  - [Converting Formats](#converting-formats)
  - [Command-line Flags](#command-line-flags)
- [🤝 Contributing](#-contributing)
- [📄 License](#-license)

</details>

## 📖 About The Project

BiliCDN is an automated tool that discovers Bilibili's CDN nodes by generating candidate domain names based on known naming patterns, verifying them via DNS resolution and HTTP checks, and categorizing the results by geographic region and cloud provider. The resulting node lists are automatically committed to the `data` branch in multiple formats.

This project aims to provide an up-to-date, reliable catalog of Bilibili CDN infrastructure for developers and network researchers.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ✨ Features

- **🤖 Fully Automated**: Updates weekly via GitHub Actions, with manual trigger support.
- **⚡️ High Performance**: Two-stage pipeline (DNS workers + HTTP workers) achieving 1900+ domains/sec.
- **🧠 Auto-tuning DNS**: Automatically benchmarks and selects the optimal DNS configuration for your network.
- **🪶 Zero Dependencies**: Pure Go stdlib — no external dependencies beyond `golang.org/x/term`.
- **🌍 Full Coverage**: 315+ city-level locations, 15+ ISPs, UPOS storage nodes, Gotcha CDN nodes, and external CDN (Akamai).
- **📝 Multi-Format Output**: Generates JSON, YAML, TXT, and Markdown with geographic region grouping.
- **🗺️ Region Classification**: Domains are automatically classified by province, city, cloud provider, and CDN type.
- **🔄 Atomic Output**: Lock files and atomic file replacement prevent data corruption.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ⚙️ How It Works

1. **Auto-tune**: Probes DNS servers, benchmarks different configurations, and selects the fastest.
2. **Generate**: Produces ~4.5M candidate domains from known Bilibili CDN naming patterns.
3. **DNS Resolve**: 1500 concurrent DNS workers verify each domain exists (NXDOMAIN filtering).
4. **HTTP Verify**: 50 HTTP workers confirm resolved nodes are alive via HEAD requests.
5. **Output**: Results are sorted, deduplicated, and written atomically. `bilicdn convert` generates region-grouped formats.
6. **Publish**: `wbxBot` commits updated data to the `data` branch weekly.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🚀 How to Use the Data

### Web Browser

Browse all CDN nodes at [**bilicdn.pages.dev**](https://bilicdn.pages.dev):

- Interactive table with search, sort, and region filtering
- Click any domain to copy it
- Download files via the "Download" button in the header
- Copy API endpoint URLs via the "API" button

### API Endpoints

All data files are served via Cloudflare CDN for fast global access:

```
https://bilicdn.pages.dev/nodes.json
https://bilicdn.pages.dev/nodes.yml
https://bilicdn.pages.dev/nodes.txt
https://bilicdn.pages.dev/nodes.md
https://bilicdn.pages.dev/domains.txt
```

### Direct Download

Or download raw data files from the [`data` branch][data-branch-link]:

| File | Description |
| --- | --- |
| `domains.txt` | Flat list, one domain per line (scanner raw output) |
| `nodes.json` | JSON grouped by region |
| `nodes.yml` | YAML grouped by region |
| `nodes.txt` | Plain text grouped by region |
| `nodes.md` | Markdown with geographic area sections |

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🛠️ Local Development

### Prerequisites

- Go 1.26+

### Running the Scanner

```bash
# Build
go build -o bilicdn .

# Run with default settings (auto-tune DNS, full scan)
./bilicdn

# Quick scan with smaller range
./bilicdn -be 3 -se 10 -gotcha=false

# Use specific DNS strategy
./bilicdn -dns 2    # CN-only DNS
./bilicdn -dns 3    # System DNS

# Custom output path
./bilicdn -o /tmp/results.txt

# CI-friendly mode (no TUI progress bar)
./bilicdn -quiet

# Resume from last checkpoint (full scan ~76 min, safe to interrupt)
./bilicdn -resume
```

### Converting Formats

```bash
# Default: domains.txt → nodes.json
./bilicdn convert

# Specify input/output
./bilicdn convert -i data/domains.txt -o data/nodes.json
./bilicdn convert -i data/domains.txt -o data/nodes.yml
./bilicdn convert -i data/domains.txt -o data/nodes.txt
./bilicdn convert -i data/domains.txt -o data/nodes.md

# Force format (ignore extension)
./bilicdn convert -o output -f yaml
```

### Command-line Flags

**Scanner:**

| Flag | Default | Description |
| --- | --- | --- |
| `-dns` | `0` | DNS strategy: 0=Auto, 1=Global, 2=CN, 3=System |
| `-c` | `0` | Concurrent workers (0 = auto) |
| `-d` | `bilivideo.com` | Target domain |
| `-o` | `data/domains.txt` | Output file path |
| `-bs` / `-be` | `1` / `10` | Block range start/end |
| `-ss` / `-se` | `1` / `50` | Server range start/end |
| `-gotcha` | `true` | Enable Gotcha pattern scanning |
| `-quiet` | `false` | Log mode (no TUI) |
| `-debug` | `false` | Write errors to scanner_errors.log |
| `-resume` | `false` | Resume from last checkpoint |

**Convert:**

| Flag | Default | Description |
| --- | --- | --- |
| `-i` | `data/domains.txt` | Input domains file |
| `-o` | `data/nodes.json` | Output file (.json/.yml/.txt/.md) |
| `-f` | (auto) | Force format (json/yaml/txt/md) |

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🤝 Contributing

Contributions are welcome! Please feel free to open an issue or submit a pull request.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 📄 License

This project is licensed under the MIT License. See the `LICENSE` file for details.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

Copyright © 2026 Babywbx.

<!-- LINK GROUP -->

[ci-shield]: https://img.shields.io/github/actions/workflow/status/babywbx/BiliCDN/ci.yml?label=CI&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[ci-link]: https://github.com/babywbx/BiliCDN/actions/workflows/ci.yml
[update-data-shield]: https://img.shields.io/github/actions/workflow/status/babywbx/BiliCDN/update-data.yml?label=Automatically%20update%20CDN%20data&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[update-data-link]: https://github.com/babywbx/BiliCDN/actions/workflows/update-data.yml
[last-updated-shield]: https://img.shields.io/github/last-commit/babywbx/BiliCDN/data?label=Last%20updated%20CDN%20data&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[last-updated-link]: https://github.com/babywbx/BiliCDN/tree/data
[github-license-shield]: https://img.shields.io/github/license/babywbx/BiliCDN?style=flat-square&logo=opensourceinitiative&labelColor=black&color=white
[github-license-link]: https://github.com/babywbx/BiliCDN/blob/main/LICENSE
[back-to-top]: https://img.shields.io/badge/-BACK_TO_TOP-151515?style=flat-square
[data-branch-link]: https://github.com/babywbx/BiliCDN/tree/data
