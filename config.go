package main

import "time"

// Configurable via flags (set in main.go)
var (
	flagConcurrency = 0 // 0 = auto (match DNS QPS capacity)
	flagDomain      = "bilivideo.com"
	flagDNSStrategy = 0 // 0=Auto, 1=Global, 2=CN, 3=System
	flagDebug       = false
	flagQuiet       = false // true = log mode (no TUI, periodic log lines)
	flagOutput      = "data/domains.txt"
	flagGotcha      = true
	flagResume      = false
	flagBlockStart  = 1
	flagBlockEnd    = 10
	flagServerStart = 1
	flagServerEnd   = 50
)

// Internal defaults
const (
	requestTimeout = 3 * time.Second
	dnsTimeout     = 400 * time.Millisecond
	maxDNSRetries  = 1
	maxHTTPRetries = 2
	maxTwoDigit    = 99

	// HTTP connection pool (sized for high concurrency)
	maxIdleConns        = 1000
	maxIdleConnsPerHost = 2
	idleConnTimeout     = 30 * time.Second

	// Performance tuning
	progressUpdateInterval = 300 * time.Millisecond
	jobBufferSize          = 8000
	httpWorkerCount        = 50 // HTTP verification workers (separate from DNS workers)

	// DNS rate limiting
	nxdomainCacheTTL = 60 * time.Second
)

// DNSServer defines a DNS server with its provider-specific QPS limit
type DNSServer struct {
	Addr string
	QPS  int
}

// DNS server tiers
var dnsOverseas = []DNSServer{
	{"8.8.8.8:53", 800},
	{"8.8.4.4:53", 800},
	{"1.1.1.1:53", 500},
	{"1.0.0.1:53", 500},
	{"4.2.2.1:53", 400},
	{"4.2.2.2:53", 400},
	{"64.6.64.6:53", 200},
	{"64.6.65.6:53", 200},
	{"9.9.9.9:53", 200},
	{"149.112.112.112:53", 200},
}

var dnsDomestic = []DNSServer{
	{"223.5.5.5:53", 200},    // AliDNS
	{"223.6.6.6:53", 200},    // AliDNS
	{"119.29.29.29:53", 150}, // DNSPod
	{"1.12.12.12:53", 150},   // DNSPod
	{"180.184.1.1:53", 100},  // ByteDance
	{"180.184.2.2:53", 100},  // ByteDance
	{"114.114.114.114:53", 100},
	{"114.114.115.115:53", 100},
}

// ---------------------------------------------------------------------------
// CDN Node Data Model
// ---------------------------------------------------------------------------

// Locations: base city codes from all provinces.
// Number-suffixed variants (bj3, tj2, etc.) are generated automatically.
var baseLocations = []string{
	// Direct municipalities
	"bj", "tj", "sh", "cq",
	// Chongqing sub-regions
	"cqwz", "cqfl", "cqqj",
	// Hebei
	"hbsjz", "hbts", "hbqhd", "hbhd", "hbxt", "hbbd", "hbzjk", "hbcd", "hbcz", "hblf", "hbhs",
	// Shanxi
	"sxty", "sxdt", "sxyq", "sxcz", "sxjc", "sxsz", "sxjz", "sxyc", "sxxz", "sxlf", "sxll",
	// Inner Mongolia
	"nmgheht", "nmghhht", "nmgbt", "nmgwh", "nmgcf", "nmgtl", "nmgeeds", "nmghle", "nmgbyne", "nmgwcb", "nmgxam", "nmgalsm",
	// Liaoning
	"lnsy", "lndl", "lnas", "lnfs", "lnbx", "lndd", "lnjz", "lnyk", "lnfy", "lnly", "lnpl", "lntl", "lncy", "lnhld",
	// Jilin
	"jlcc", "jljl", "jlsp", "jlly", "jlth", "jlbs", "jlby", "jlcb",
	// Heilongjiang
	"hljheb", "hljqqhr", "hljjx", "hljhg", "hljsy", "hljyt", "hljjms", "hljqqhe", "hljqth", "hljmdj", "hljsh", "hljdxal",
	// Jiangsu
	"jsnj", "jswx", "jsxz", "jscz", "jssz", "jsnt", "jslyg", "jsha", "jsyc", "jsyz", "jszj", "jstz", "jssq",
	// Zhejiang
	"zjhz", "zjnb", "zjwz", "zjjx", "zjhuz", "zjsx", "zjjh", "zjqz", "zjzs", "zjtz", "zjls",
	// Anhui
	"ahhf", "ahwh", "ahbb", "ahhn", "ahmas", "ahhb", "ahtl", "ahan", "ahfy", "ahsz", "ahch", "ahla", "ahbz", "ahcz", "ahxuanc",
	// Fujian
	"fjfz", "fjxm", "fjpt", "fjsm", "fjqz", "fjzz", "fjnp", "fjly", "fjnd",
	// Jiangxi
	"jxnc", "jxjj", "jxjdz", "jxpx", "jxxiny", "jxyt", "jxganz", "jxja", "jxyc", "jxfz", "jxsr",
	// Shandong
	"sdjn", "sdqd", "sdzb", "sdzz", "sddy", "sdyt", "sdwf", "sdta", "sdwh", "sdrz", "sdlw", "sdlc", "sddz", "sdhz", "sdbz",
	// Henan
	"hnzz", "hnkf", "hnly", "hnpds", "hnay", "hnhb", "hnxx", "hnjz", "hnpy", "hnxc", "hnlh", "hnsmx", "hnny", "hnsq", "hnxy", "hnzk", "hnzmd",
	// Hubei
	"hbwh", "hbhg", "hbyc", "hbsh", "hbxf", "hbjz", "hbxn", "hbez", "hbbz", "hbxian",
	// Hunan
	"hncs", "hnxt", "hnhy", "hnyy", "hncz", "hncd", "hnld", "hnhh", "hnxxz",
	// Guangdong
	"gdgz", "gdsz", "gdzh", "gdst", "gdfs", "gdsg", "gdzj", "gdjm", "gdzq", "gdmm", "gdmz", "gdsw", "gdhy", "gdyj", "gddg", "gdzs", "gdcz", "gdqy", "gdyf",
	// Guangxi
	"gxnn", "gxlz", "gxgl", "gxwz", "gxbh", "gxfcg", "gxqinz", "gxgg", "gxyl", "gxbs", "gxhz", "gxhc", "gxlb", "gxcz",
	// Hainan
	"hnhk", "hnsy", "hnsans", "hndz",
	// Sichuan
	"sccd", "sczg", "scpanzh", "scdz", "scmy", "scgy", "scsn", "scnc", "scms", "scyb", "scnj", "scls", "scdy", "scbaz", "scganz", "sclsz",
	// Guizhou
	"gzgy", "gzlps", "gzzy", "gzan", "gzbj", "gztr", "gzqxn", "gzqdn",
	// Yunnan
	"ynkm", "ynqj", "ynys", "ynbshan", "ynzt", "ynhh", "ynpe", "ynlinc", "yncx", "yndh", "yndq", "ynls", "ynxsbn",
	// Tibet
	"xzls", "xzrkz", "xzcd", "xzlh", "xzsn", "xznq", "xzali",
	// Shaanxi
	"sxxa", "sxtc", "sxbj", "sxxy", "sxwn", "sxyan", "sxhz", "sxyl", "sxak", "sxsl",
	// Gansu
	"gslz", "gsjyg", "gsjinch", "gsbaiy", "gsts", "gsww", "gszy", "gspl", "gsqy", "gsdx", "gsln", "gslinx", "gsgann",
	// Qinghai
	"qhxn", "qhhd", "qhhb", "qhhnan", "qhhx", "qhys", "qhgy", "qhge",
	// Ningxia
	"nxyc", "nxszs", "nxwz", "nxgy", "nxzw",
	// Xinjiang
	"xjwlmq", "xjklmy", "xjtlf", "xjhm", "xjbt", "xjaks", "xjks", "xjkz", "xjht", "xjalt", "xjtc", "xjcj",
	// Hong Kong (found in real data)
	"hk",
}

// Known number-suffixed location variants discovered from real CDN data.
// These are separate data centers in the same city.
var numberedLocations = []string{
	"bj3",
	"hbsjz2",
	"hbyc2",
	"hncs3",
	"jlcc3",
	"jsnj2",
	"sccd3",
	"sdjn2",
	"tj2",
	"zjhz2", "zjhz3", "zjhz4",
	"zjjh4",
}

// ISPs for standard nodes: cn-{loc}-{isp}-{block}-{server}
var standardISPs = []string{
	"cm", "cmcc", "ct", "cu", "dx", // Major carriers
	"txy", "ali", "hw", "ks3", "bdy", // Cloud providers
	"qn", "fx", "gd", "cc", // CDN vendors
	"se", // Special edge
}

// ISPs observed in bcache/v/live nodes (superset — includes niche ISPs)
var extendedISPs = []string{
	"cm", "cmcc", "ct", "cu", "dx",
	"txy", "ali", "hw", "ks3", "bdy",
	"qn", "fx", "gd", "cc", "se",
	"ix",     // Internet exchange
	"bn",     // Broadnet
	"wasu",   // WaSu Media (Zhejiang)
	"twsx",   // Shenzhen Topway (Guangdong cable)
	"eq",     // Equinix
	"ccc",    // China Cache (variant)
	"office", // Internal/office network
}

// NodeType defines a CDN node category for domain generation
type NodeType struct {
	Name   string   // type segment in domain name (e.g. "bcache", "v", "live")
	ISPs   []string // which ISP list to use
	MaxNum int      // max server number to enumerate (1..MaxNum)
}

// CDN node types discovered from comprehensive data analysis
var nodeTypes = []NodeType{
	{Name: "standard", ISPs: standardISPs, MaxNum: 0}, // uses flagBlockStart/End, flagServerStart/End
	{Name: "bcache", ISPs: extendedISPs, MaxNum: 25},
	{Name: "v", ISPs: extendedISPs, MaxNum: 25},
	{Name: "live", ISPs: extendedISPs, MaxNum: 10},
}

// ---------------------------------------------------------------------------
// Gotcha patterns
// ---------------------------------------------------------------------------

// Gotcha prefixes: {letter}{optional_number} (c0, c1, d0, d1 are confirmed)
var gotchaPrefixes = []string{
	"a", "a1", "a2", "a3", "a4", "a5",
	"b", "b1", "b2", "b3", "b4", "b5",
	"c", "c0", "c1", "c2", "c3", "c4", "c5",
	"d", "d0", "d1", "d2", "d3", "d4", "d5",
}

// Gotcha middle parts: empty or p{N}
var gotchaMiddles = []string{"", "p1", "p2", "p3", "p4", "p5"}

// Gotcha regions
var gotchaRegions = []string{"cn", "ov", "tf"}

// Gotcha number range
const (
	gotchaNumberStart = 0
	gotchaNumberEnd   = 350
)

// Gotcha suffixes
var gotchaSuffixes = []string{"", "b", "-1", "-2", "-3", "-4", "-5", "-basic", "-loc"}

// ---------------------------------------------------------------------------
// UPOS storage/upload nodes
// ---------------------------------------------------------------------------

// UPOS prefixes and components discovered from real data.
// Pattern: upos-{region}-{type}{provider}{variant}.bilivideo.com
var uposNodes = []string{
	// Mirror nodes (CDN edge caches for video delivery)
	"upos-sz-mirrorali", "upos-sz-mirrorali02", "upos-sz-mirroralib",
	"upos-sz-mirroralibstar1", "upos-sz-mirroraliov",
	"upos-sz-mirrorasiabstar1", "upos-sz-mirrorasiaov",
	"upos-sz-mirrorbd",
	"upos-sz-mirrorcf1ov",
	"upos-sz-mirrorcos", "upos-sz-mirrorcosb", "upos-sz-mirrorcosbstar", "upos-sz-mirrorcosbstar1",
	"upos-sz-mirrorcosdisp", "upos-sz-mirrorcoso1", "upos-sz-mirrorcosov",
	"upos-sz-mirrorctos",
	"upos-sz-mirrorhw", "upos-sz-mirrorhwb", "upos-sz-mirrorhwdisp",
	"upos-sz-mirror08c", "upos-sz-mirror08h",
	// Origin nodes (source storage)
	"upos-sz-originbstar", "upos-sz-origincosgzhw", "upos-sz-origincosv",
	// Static content
	"upos-sz-static", "upos-sz-staticcos",
	// Upload CDN nodes
	"upos-sz-upcdnakam", "upos-sz-upcdnbda2", "upos-sz-upcdnws",
	"upos-cs-upcdnbda", "upos-cs-upcdnbldsa",
	// Edge storage
	"upos-sz-estgcos", "upos-sz-estghw", "upos-sz-estgoss", "upos-sz-estgoss02",
	// Other regions
	"upos-bstar2-mirrorakam",
	"upos-sz-dynqn",
	// P2P/302 (included for completeness — may be inactive)
	"upos-sz-302kodo",
}

// Misc service subdomains (non-CDN but part of bilivideo.com infrastructure)
var miscNodes = []string{
	"api", "bvc", "bvc-drm", "core", "data", "activity", "cloudapp",
	"openupos", "qoe", "skynet", "transfer", "vanadium",
	"proxy-tf-all-ws", "bfs-tf-all-js", "txy-fmp4hls",
	"jscs-luffy-upcdntx",
	"live-push", "bdy.live-push", "txy2.live-push", "txy3.live-push",
}

// External CDN domains not under bilivideo.com (stored as full domains)
var externalNodes = []string{
	"upos-hz-mirrorakam.akamaized.net",
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// allLocations returns base + numbered location codes, deduplicated
func allLocations() []string {
	seen := make(map[string]struct{}, len(baseLocations)+len(numberedLocations))
	result := make([]string, 0, len(baseLocations)+len(numberedLocations))
	for _, lists := range [2][]string{baseLocations, numberedLocations} {
		for _, loc := range lists {
			if _, ok := seen[loc]; !ok {
				seen[loc] = struct{}{}
				result = append(result, loc)
			}
		}
	}
	return result
}
