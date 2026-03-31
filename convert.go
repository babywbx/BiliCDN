package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Region mapping: location code prefix → "省份-城市".
// Covers all baseLocations from config.go.
var regionMap = map[string]string{
	// Direct municipalities
	"bj": "北京", "tj": "天津", "sh": "上海", "cq": "重庆",
	"cqwz": "重庆-万州", "cqfl": "重庆-涪陵", "cqqj": "重庆-黔江",
	// Hebei
	"hbsjz": "河北-石家庄", "hbts": "河北-唐山", "hbqhd": "河北-秦皇岛", "hbhd": "河北-邯郸", "hbxt": "河北-邢台",
	"hbbd": "河北-保定", "hbzjk": "河北-张家口", "hbcd": "河北-承德", "hbcz": "河北-沧州", "hblf": "河北-廊坊", "hbhs": "河北-衡水",
	// Shanxi (晋)
	"sxty": "山西-太原", "sxdt": "山西-大同", "sxyq": "山西-阳泉", "sxcz": "山西-长治", "sxjc": "山西-晋城",
	"sxsz": "山西-朔州", "sxjz": "山西-晋中", "sxyc": "山西-运城", "sxxz": "山西-忻州", "sxlf": "山西-临汾", "sxll": "山西-吕梁",
	// Inner Mongolia
	"nmgheht": "内蒙古-呼和浩特", "nmghhht": "内蒙古-呼和浩特", "nmgbt": "内蒙古-包头", "nmgwh": "内蒙古-乌海",
	"nmgcf": "内蒙古-赤峰", "nmgtl": "内蒙古-通辽", "nmgeeds": "内蒙古-鄂尔多斯", "nmghle": "内蒙古-呼伦贝尔",
	"nmgbyne": "内蒙古-巴彦淖尔", "nmgwcb": "内蒙古-乌兰察布", "nmgxam": "内蒙古-锡林郭勒", "nmgalsm": "内蒙古-阿拉善",
	// Liaoning
	"lnsy": "辽宁-沈阳", "lndl": "辽宁-大连", "lnas": "辽宁-鞍山", "lnfs": "辽宁-抚顺", "lnbx": "辽宁-本溪",
	"lndd": "辽宁-丹东", "lnjz": "辽宁-锦州", "lnyk": "辽宁-营口", "lnfy": "辽宁-阜新", "lnly": "辽宁-辽阳",
	"lnpl": "辽宁-盘锦", "lntl": "辽宁-铁岭", "lncy": "辽宁-朝阳", "lnhld": "辽宁-葫芦岛",
	// Jilin
	"jlcc": "吉林-长春", "jljl": "吉林-吉林", "jlsp": "吉林-四平", "jlly": "吉林-辽源",
	"jlth": "吉林-通化", "jlbs": "吉林-白山", "jlby": "吉林-白城", "jlcb": "吉林-长白山",
	// Heilongjiang
	"hljheb": "黑龙江-哈尔滨", "hljqqhr": "黑龙江-齐齐哈尔", "hljjx": "黑龙江-鸡西", "hljhg": "黑龙江-鹤岗",
	"hljsy": "黑龙江-双鸭山", "hljyt": "黑龙江-伊春", "hljjms": "黑龙江-佳木斯", "hljqqhe": "黑龙江-七台河",
	"hljqth": "黑龙江-七台河", "hljmdj": "黑龙江-牡丹江", "hljsh": "黑龙江-绥化", "hljdxal": "黑龙江-大兴安岭",
	// Jiangsu
	"jsnj": "江苏-南京", "jswx": "江苏-无锡", "jsxz": "江苏-徐州", "jscz": "江苏-常州", "jssz": "江苏-苏州",
	"jsnt": "江苏-南通", "jslyg": "江苏-连云港", "jsha": "江苏-淮安", "jsyc": "江苏-盐城", "jsyz": "江苏-扬州",
	"jszj": "江苏-镇江", "jstz": "江苏-泰州", "jssq": "江苏-宿迁",
	// Zhejiang
	"zjhz": "浙江-杭州", "zjnb": "浙江-宁波", "zjwz": "浙江-温州", "zjjx": "浙江-嘉兴", "zjhuz": "浙江-湖州",
	"zjsx": "浙江-绍兴", "zjjh": "浙江-金华", "zjqz": "浙江-衢州", "zjzs": "浙江-舟山", "zjtz": "浙江-台州", "zjls": "浙江-丽水",
	// Anhui
	"ahhf": "安徽-合肥", "ahwh": "安徽-芜湖", "ahbb": "安徽-蚌埠", "ahhn": "安徽-淮南", "ahmas": "安徽-马鞍山",
	"ahhb": "安徽-淮北", "ahtl": "安徽-铜陵", "ahan": "安徽-安庆", "ahfy": "安徽-阜阳", "ahsz": "安徽-宿州",
	"ahch": "安徽-滁州", "ahla": "安徽-六安", "ahbz": "安徽-亳州", "ahcz": "安徽-池州", "ahxuanc": "安徽-宣城",
	// Fujian
	"fjfz": "福建-福州", "fjxm": "福建-厦门", "fjpt": "福建-莆田", "fjsm": "福建-三明", "fjqz": "福建-泉州",
	"fjzz": "福建-漳州", "fjnp": "福建-南平", "fjly": "福建-龙岩", "fjnd": "福建-宁德",
	// Jiangxi
	"jxnc": "江西-南昌", "jxjj": "江西-九江", "jxjdz": "江西-景德镇", "jxpx": "江西-萍乡", "jxxiny": "江西-新余",
	"jxyt": "江西-鹰潭", "jxganz": "江西-赣州", "jxja": "江西-吉安", "jxyc": "江西-宜春", "jxfz": "江西-抚州", "jxsr": "江西-上饶",
	// Shandong
	"sdjn": "山东-济南", "sdqd": "山东-青岛", "sdzb": "山东-淄博", "sdzz": "山东-枣庄", "sddy": "山东-东营",
	"sdyt": "山东-烟台", "sdwf": "山东-潍坊", "sdta": "山东-泰安", "sdwh": "山东-威海", "sdrz": "山东-日照",
	"sdlw": "山东-莱芜", "sdlc": "山东-临沂", "sddz": "山东-德州", "sdhz": "山东-菏泽", "sdbz": "山东-滨州",
	// Henan
	"hnzz": "河南-郑州", "hnkf": "河南-开封", "hnly": "河南-洛阳", "hnpds": "河南-平顶山", "hnay": "河南-安阳",
	"hnhb": "河南-鹤壁", "hnxx": "河南-新乡", "hnjz": "河南-焦作", "hnpy": "河南-濮阳", "hnxc": "河南-许昌",
	"hnlh": "河南-漯河", "hnsmx": "河南-三门峡", "hnny": "河南-南阳", "hnsq": "河南-商丘", "hnxy": "河南-信阳",
	"hnzk": "河南-周口", "hnzmd": "河南-驻马店",
	// Hubei
	"hbwh": "湖北-武汉", "hbhg": "湖北-黄冈", "hbyc": "湖北-宜昌", "hbsh": "湖北-十堰", "hbxf": "湖北-襄阳",
	"hbjz": "湖北-荆州", "hbxn": "湖北-咸宁", "hbez": "湖北-恩施", "hbbz": "湖北-黄石", "hbxian": "湖北-仙桃",
	// Hunan
	"hncs": "湖南-长沙", "hnxt": "湖南-湘潭", "hnhy": "湖南-衡阳", "hnyy": "湖南-岳阳", "hncz": "湖南-常德",
	"hncd": "湖南-郴州", "hnld": "湖南-娄底", "hnhh": "湖南-怀化", "hnxxz": "湖南-湘西",
	// Guangdong
	"gdgz": "广东-广州", "gdsz": "广东-深圳", "gdzh": "广东-珠海", "gdst": "广东-汕头", "gdfs": "广东-佛山",
	"gdsg": "广东-韶关", "gdzj": "广东-湛江", "gdjm": "广东-江门", "gdzq": "广东-肇庆", "gdmm": "广东-茂名",
	"gdmz": "广东-梅州", "gdsw": "广东-汕尾", "gdhy": "广东-河源", "gdyj": "广东-阳江", "gddg": "广东-东莞",
	"gdzs": "广东-中山", "gdcz": "广东-潮州", "gdqy": "广东-清远", "gdyf": "广东-云浮",
	// Guangxi
	"gxnn": "广西-南宁", "gxlz": "广西-柳州", "gxgl": "广西-桂林", "gxwz": "广西-梧州", "gxbh": "广西-北海",
	"gxfcg": "广西-防城港", "gxqinz": "广西-钦州", "gxgg": "广西-贵港", "gxyl": "广西-玉林", "gxbs": "广西-百色",
	"gxhz": "广西-贺州", "gxhc": "广西-河池", "gxlb": "广西-来宾", "gxcz": "广西-崇左",
	// Hainan
	"hnhk": "海南-海口", "hnsy": "海南-三亚", "hnsans": "海南-三沙", "hndz": "海南-儋州",
	// Sichuan
	"sccd": "四川-成都", "sczg": "四川-自贡", "scpanzh": "四川-攀枝花", "scdz": "四川-达州", "scmy": "四川-绵阳",
	"scgy": "四川-广元", "scsn": "四川-遂宁", "scnc": "四川-南充", "scms": "四川-眉山", "scyb": "四川-宜宾",
	"scnj": "四川-内江", "scls": "四川-乐山", "scdy": "四川-德阳", "scbaz": "四川-巴中", "scganz": "四川-甘孜", "sclsz": "四川-凉山",
	// Guizhou
	"gzgy": "贵州-贵阳", "gzlps": "贵州-六盘水", "gzzy": "贵州-遵义", "gzan": "贵州-安顺",
	"gzbj": "贵州-毕节", "gztr": "贵州-铜仁", "gzqxn": "贵州-黔西南", "gzqdn": "贵州-黔东南",
	// Yunnan
	"ynkm": "云南-昆明", "ynqj": "云南-曲靖", "ynys": "云南-玉溪", "ynbshan": "云南-保山", "ynzt": "云南-昭通",
	"ynhh": "云南-红河", "ynpe": "云南-普洱", "ynlinc": "云南-临沧", "yncx": "云南-楚雄", "yndh": "云南-大理",
	"yndq": "云南-德宏", "ynls": "云南-丽江", "ynxsbn": "云南-西双版纳",
	// Tibet
	"xzls": "西藏-拉萨", "xzrkz": "西藏-日喀则", "xzcd": "西藏-昌都", "xzlh": "西藏-林芝",
	"xzsn": "西藏-山南", "xznq": "西藏-那曲", "xzali": "西藏-阿里",
	// Shaanxi (陕)
	"sxxa": "陕西-西安", "sxtc": "陕西-铜川", "sxbj": "陕西-宝鸡", "sxxy": "陕西-咸阳", "sxwn": "陕西-渭南",
	"sxyan": "陕西-延安", "sxhz": "陕西-汉中", "sxyl": "陕西-榆林", "sxak": "陕西-安康", "sxsl": "陕西-商洛",
	// Gansu
	"gslz": "甘肃-兰州", "gsjyg": "甘肃-嘉峪关", "gsjinch": "甘肃-金昌", "gsbaiy": "甘肃-白银", "gsts": "甘肃-天水",
	"gsww": "甘肃-武威", "gszy": "甘肃-张掖", "gspl": "甘肃-平凉", "gsqy": "甘肃-庆阳", "gsdx": "甘肃-定西",
	"gsln": "甘肃-陇南", "gslinx": "甘肃-临夏", "gsgann": "甘肃-甘南",
	// Qinghai
	"qhxn": "青海-西宁", "qhhd": "青海-海东", "qhhb": "青海-海北", "qhhnan": "青海-海南州",
	"qhhx": "青海-海西", "qhys": "青海-玉树", "qhgy": "青海-果洛", "qhge": "青海-格尔木",
	// Ningxia
	"nxyc": "宁夏-银川", "nxszs": "宁夏-石嘴山", "nxwz": "宁夏-吴忠", "nxgy": "宁夏-固原", "nxzw": "宁夏-中卫",
	// Xinjiang
	"xj":     "新疆",
	"xjwlmq": "新疆-乌鲁木齐", "xjklmy": "新疆-克拉玛依", "xjtlf": "新疆-吐鲁番", "xjhm": "新疆-哈密",
	"xjbt": "新疆-博尔塔拉", "xjaks": "新疆-阿克苏", "xjks": "新疆-喀什", "xjkz": "新疆-克孜勒苏",
	"xjht": "新疆-和田", "xjalt": "新疆-阿勒泰", "xjtc": "新疆-塔城", "xjcj": "新疆-昌吉",
	// Hong Kong
	"hk": "香港",
}

// UPOS cloud provider classification
var uposProviderMap = map[string]string{
	"ali":  "阿里云",
	"cos":  "腾讯云",
	"hw":   "华为云",
	"bd":   "百度云",
	"akam": "Akamai",
	"cf":   "Cloudflare",
	"kodo": "七牛云",
	"qn":   "七牛云",
	"ctos": "天翼云",
	"ks":   "金山云",
}

// classifyDomain assigns a domain to a region category.
func classifyDomain(domain string) string {
	// Strip the TLD suffix for classification (handle both bilivideo.com and akamaized.net)
	name := domain
	if idx := strings.Index(name, "."); idx > 0 {
		name = name[:idx]
	}

	if strings.HasPrefix(name, "upos-") {
		// Classify by cloud provider: match longest provider key in domain
		bestProvider := ""
		bestKey := ""
		for key, provider := range uposProviderMap {
			if strings.Contains(name, key) && len(key) > len(bestKey) {
				bestProvider = provider
				bestKey = key
			}
		}
		if bestProvider != "" {
			return "UPOS-" + bestProvider
		}
		return "UPOS-其他"
	}

	if strings.Contains(name, "-gotcha") {
		if strings.Contains(name, "-ov-") || strings.Contains(name, "ov-gotcha") {
			return "Gotcha-海外"
		}
		if strings.Contains(name, "-tf-") {
			return "Gotcha-TF"
		}
		return "Gotcha-国内"
	}

	if strings.HasPrefix(name, "cn-") {
		rest := name[3:]
		bestKey := ""
		for key := range regionMap {
			if strings.HasPrefix(rest, key) && len(key) > len(bestKey) {
				bestKey = key
			}
		}
		if bestKey != "" {
			return regionMap[bestKey]
		}
		if idx := strings.Index(rest, "-"); idx > 0 {
			return "cn-" + rest[:idx]
		}
	}

	return "其他"
}

// Region sort order by geographic area
var regionOrder = []string{
	// Direct municipalities
	"北京", "天津", "上海", "重庆",
	// North China
	"河北", "山西", "内蒙古",
	// Northeast
	"辽宁", "吉林", "黑龙江",
	// East China
	"江苏", "浙江", "安徽", "福建", "江西", "山东",
	// Central China
	"河南", "湖北", "湖南",
	// South China
	"广东", "广西", "海南",
	// Southwest
	"四川", "贵州", "云南", "西藏",
	// Northwest
	"陕西", "甘肃", "青海", "宁夏", "新疆",
	// SAR
	"香港",
	// CDN infra
	"UPOS", "Gotcha",
	// Fallback
	"其他",
}

// regionSortKey returns a sort key "groupOrder-regionName" for stable ordering.
func regionSortKey(region string) string {
	// Exact match (直辖市, 香港, 其他)
	for i, prefix := range regionOrder {
		if region == prefix {
			return fmt.Sprintf("%03d-%s", i, region)
		}
	}
	// Prefix match (省份-城市, UPOS-xxx, Gotcha-xxx)
	for i, prefix := range regionOrder {
		if strings.HasPrefix(region, prefix) {
			return fmt.Sprintf("%03d-%s", i, region)
		}
	}
	// Unknown: sort last
	return fmt.Sprintf("999-%s", region)
}

// sortedRegions returns region names grouped by geographic area.
func sortedRegions(grouped map[string][]string) []string {
	regions := make([]string, 0, len(grouped))
	for r := range grouped {
		regions = append(regions, r)
	}
	slices.SortFunc(regions, func(a, b string) int {
		return strings.Compare(regionSortKey(a), regionSortKey(b))
	})
	return regions
}

func runConvert(args []string) error {
	fs := flag.NewFlagSet("bilicdn convert", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	input := fs.String("i", "data/domains.txt", "Input domains file")
	output := fs.String("o", "data/nodes.json", "Output file (.json/.yml/.txt/.md)")
	format := fs.String("f", "", "Output format (json/yaml/txt/md), auto-detected from extension")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: bilicdn convert [flags]")
		fmt.Fprintln(os.Stderr, "  Convert flat domain list to region-grouped output.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Formats:")
		fmt.Fprintln(os.Stderr, "  json   JSON object grouped by region")
		fmt.Fprintln(os.Stderr, "  yaml   YAML grouped by region")
		fmt.Fprintln(os.Stderr, "  txt    Plain text with region headers")
		fmt.Fprintln(os.Stderr, "  md     Markdown with region sections")
		fmt.Fprintln(os.Stderr, "")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Detect format from extension if not specified
	fmt_ := *format
	if fmt_ == "" {
		switch strings.ToLower(filepath.Ext(*output)) {
		case ".json":
			fmt_ = "json"
		case ".yml", ".yaml":
			fmt_ = "yaml"
		case ".txt":
			fmt_ = "txt"
		case ".md", ".markdown":
			fmt_ = "md"
		default:
			fmt_ = "json"
		}
	}

	// Read input
	f, err := os.Open(*input)
	if err != nil {
		return fmt.Errorf("open %s: %w", *input, err)
	}
	defer f.Close()

	grouped := make(map[string][]string)
	scanner := bufio.NewScanner(f)
	total := 0
	for scanner.Scan() {
		domain := strings.TrimSpace(scanner.Text())
		if domain == "" {
			continue
		}
		region := classifyDomain(domain)
		grouped[region] = append(grouped[region], domain)
		total++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", *input, err)
	}

	for region := range grouped {
		slices.Sort(grouped[region])
	}

	// Render
	var data []byte
	switch fmt_ {
	case "json":
		data, err = renderJSON(grouped)
	case "yaml":
		data = renderYAML(grouped)
	case "txt":
		data = renderTXT(grouped)
	case "md":
		data = renderMD(grouped)
	default:
		return fmt.Errorf("unknown format %q (use json/yaml/txt/md)", fmt_)
	}
	if err != nil {
		return err
	}

	if dir := filepath.Dir(*output); dir != "." {
		os.MkdirAll(dir, 0o755)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", *output, err)
	}

	fmt.Fprintf(os.Stderr, "Converted %d domains → %d regions → %s (%s)\n", total, len(grouped), *output, fmt_)
	return nil
}

func renderJSON(grouped map[string][]string) ([]byte, error) {
	var b strings.Builder
	regions := sortedRegions(grouped)
	b.WriteString("{\n")
	for i, region := range regions {
		key, _ := json.Marshal(region)
		b.WriteString("  ")
		b.Write(key)
		b.WriteString(": [\n")
		domains := grouped[region]
		for j, d := range domains {
			val, _ := json.Marshal(d)
			b.WriteString("    ")
			b.Write(val)
			if j < len(domains)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString("  ]")
		if i < len(regions)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

func renderYAML(grouped map[string][]string) []byte {
	var b strings.Builder
	for _, region := range sortedRegions(grouped) {
		domains := grouped[region]
		b.WriteString(region)
		b.WriteString(":\n")
		for _, d := range domains {
			b.WriteString("  - ")
			b.WriteString(d)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func renderTXT(grouped map[string][]string) []byte {
	var b strings.Builder
	for i, region := range sortedRegions(grouped) {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(region)
		b.WriteByte('\n')
		for _, d := range grouped[region] {
			b.WriteString(d)
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

// mdAreaGroup defines a display section in the markdown output.
type mdAreaGroup struct {
	icon  string
	title string
	match func(string) bool
}

var mdAreaGroups = []mdAreaGroup{
	{"🏙️", "直辖市", func(r string) bool {
		return r == "北京" || r == "天津" || r == "上海" || r == "重庆" || strings.HasPrefix(r, "重庆-")
	}},
	{"🌏", "华北", func(r string) bool {
		return strings.HasPrefix(r, "河北") || strings.HasPrefix(r, "山西") || strings.HasPrefix(r, "内蒙古")
	}},
	{"❄️", "东北", func(r string) bool {
		return strings.HasPrefix(r, "辽宁") || strings.HasPrefix(r, "吉林") || strings.HasPrefix(r, "黑龙江")
	}},
	{"🌊", "华东", func(r string) bool {
		return strings.HasPrefix(r, "江苏") || strings.HasPrefix(r, "浙江") || strings.HasPrefix(r, "安徽") ||
			strings.HasPrefix(r, "福建") || strings.HasPrefix(r, "江西") || strings.HasPrefix(r, "山东")
	}},
	{"🏔️", "华中", func(r string) bool {
		return strings.HasPrefix(r, "河南") || strings.HasPrefix(r, "湖北") || strings.HasPrefix(r, "湖南")
	}},
	{"🌴", "华南", func(r string) bool {
		return strings.HasPrefix(r, "广东") || strings.HasPrefix(r, "广西") || strings.HasPrefix(r, "海南")
	}},
	{"🏯", "西南", func(r string) bool {
		return strings.HasPrefix(r, "四川") || strings.HasPrefix(r, "贵州") || strings.HasPrefix(r, "云南") || strings.HasPrefix(r, "西藏")
	}},
	{"🏜️", "西北", func(r string) bool {
		return strings.HasPrefix(r, "陕西") || strings.HasPrefix(r, "甘肃") || strings.HasPrefix(r, "青海") ||
			strings.HasPrefix(r, "宁夏") || strings.HasPrefix(r, "新疆")
	}},
	{"🇭🇰", "特别行政区", func(r string) bool { return r == "香港" || r == "澳门" }},
	{"☁️", "UPOS 云存储", func(r string) bool { return strings.HasPrefix(r, "UPOS") }},
	{"🔗", "Gotcha 外部 CDN", func(r string) bool { return strings.HasPrefix(r, "Gotcha") }},
	{"📦", "其他", func(r string) bool { return true }},
}

// mdTable writes a compact markdown table (no padding alignment).
func mdTable(b *strings.Builder, headers []string, rows [][]string) {
	// Header
	b.WriteByte('|')
	for _, h := range headers {
		fmt.Fprintf(b, " %s |", h)
	}
	b.WriteByte('\n')

	// Separator
	b.WriteByte('|')
	for range headers {
		b.WriteString(" --- |")
	}
	b.WriteByte('\n')

	// Rows
	for _, row := range rows {
		b.WriteByte('|')
		for _, cell := range row {
			fmt.Fprintf(b, " %s |", cell)
		}
		b.WriteByte('\n')
	}
}

func renderMD(grouped map[string][]string) []byte {
	var b strings.Builder
	regions := sortedRegions(grouped)

	totalDomains := 0
	for _, r := range regions {
		totalDomains += len(grouped[r])
	}

	b.WriteString("# BiliCDN 节点列表\n\n")
	b.WriteString(fmt.Sprintf("> 共 **%d** 个节点，**%d** 个分区\n\n", totalDomains, len(regions)))

	// Build per-area sections
	used := make(map[string]bool)
	first := true
	for _, area := range mdAreaGroups {
		var areaRegions []string
		areaCount := 0
		for _, r := range regions {
			if !used[r] && area.match(r) {
				areaRegions = append(areaRegions, r)
				areaCount += len(grouped[r])
				used[r] = true
			}
		}
		if len(areaRegions) == 0 {
			continue
		}

		if !first {
			b.WriteByte('\n')
		}
		first = false

		b.WriteString(fmt.Sprintf("## %s %s (%d)\n\n", area.icon, area.title, areaCount))

		for i, region := range areaRegions {
			domains := grouped[region]
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(fmt.Sprintf("### %s\n\n", region))

			rows := make([][]string, len(domains))
			for j, d := range domains {
				rows[j] = []string{fmt.Sprintf("%d", j+1), "`" + d + "`"}
			}
			mdTable(&b, []string{"#", "域名"}, rows)
		}
	}

	// Trim trailing whitespace
	return []byte(strings.TrimRight(b.String(), "\n") + "\n")
}
