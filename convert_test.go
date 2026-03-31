package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyDomainStandard(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"cn-bj-fx-01-04.bilivideo.com", "北京"},
		{"cn-sh-ct-01-06.bilivideo.com", "上海"},
		{"cn-tj-cu-01-02.bilivideo.com", "天津"},
		{"cn-sccd-ct-01-02.bilivideo.com", "四川-成都"},
		{"cn-fjfz-fx-01-01.bilivideo.com", "福建-福州"},
		{"cn-hk-eq-01-01.bilivideo.com", "香港"},
		{"cn-nmghhht-cu-01-01.bilivideo.com", "内蒙古-呼和浩特"},
		{"cn-xj-ct-01-01.bilivideo.com", "新疆"},
		{"cn-xjwlmq-ct-01-01.bilivideo.com", "新疆-乌鲁木齐"},
		{"cn-hljheb-ct-01-02.bilivideo.com", "黑龙江-哈尔滨"},
	}
	for _, tt := range tests {
		got := classifyDomain(tt.domain)
		if got != tt.want {
			t.Errorf("classifyDomain(%q) = %q, want %q", tt.domain, got, tt.want)
		}
	}
}

func TestClassifyDomainUPOS(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"upos-sz-mirroraliov.bilivideo.com", "UPOS-阿里云"},
		{"upos-sz-mirrorcosov.bilivideo.com", "UPOS-腾讯云"},
		{"upos-sz-mirrorhwdisp.bilivideo.com", "UPOS-华为云"},
		{"upos-sz-mirrorbd.bilivideo.com", "UPOS-百度云"},
		{"upos-hz-mirrorakam.akamaized.net", "UPOS-Akamai"},
		{"upos-sz-mirrorcf1ov.bilivideo.com", "UPOS-Cloudflare"},
		{"upos-sz-302kodo.bilivideo.com", "UPOS-七牛云"},
		{"upos-sz-dynqn.bilivideo.com", "UPOS-七牛云"},
		{"upos-sz-mirrorctos.bilivideo.com", "UPOS-天翼云"},
		{"upos-sz-static.bilivideo.com", "UPOS-其他"},
	}
	for _, tt := range tests {
		got := classifyDomain(tt.domain)
		if got != tt.want {
			t.Errorf("classifyDomain(%q) = %q, want %q", tt.domain, got, tt.want)
		}
	}
}

func TestClassifyDomainGotcha(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"c0--cn-gotcha01.bilivideo.com", "Gotcha-国内"},
		{"d1--cn-gotcha207.bilivideo.com", "Gotcha-国内"},
		{"d1--ov-gotcha03.bilivideo.com", "Gotcha-海外"},
		{"d1--tf-gotcha04.bilivideo.com", "Gotcha-TF"},
	}
	for _, tt := range tests {
		got := classifyDomain(tt.domain)
		if got != tt.want {
			t.Errorf("classifyDomain(%q) = %q, want %q", tt.domain, got, tt.want)
		}
	}
}

func TestClassifyDomainOther(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"api.bilivideo.com", "其他"},
		{"bvc.bilivideo.com", "其他"},
	}
	for _, tt := range tests {
		got := classifyDomain(tt.domain)
		if got != tt.want {
			t.Errorf("classifyDomain(%q) = %q, want %q", tt.domain, got, tt.want)
		}
	}
}

func TestClassifyDomainUnknownLocation(t *testing.T) {
	got := classifyDomain("cn-zzz-ct-01-01.bilivideo.com")
	if got != "cn-zzz" {
		t.Errorf("unknown location: got %q, want %q", got, "cn-zzz")
	}
}

func TestRegionSortOrder(t *testing.T) {
	grouped := map[string][]string{
		"其他":        {"api.bilivideo.com"},
		"UPOS-阿里云":  {"upos-sz-mirroraliov.bilivideo.com"},
		"Gotcha-海外": {"d1--ov-gotcha03.bilivideo.com"},
		"北京":        {"cn-bj-fx-01-04.bilivideo.com"},
		"四川-成都":     {"cn-sccd-ct-01-02.bilivideo.com"},
		"广东-广州":     {"cn-gdgz-fx-01-01.bilivideo.com"},
		"香港":        {"cn-hk-eq-01-01.bilivideo.com"},
	}

	regions := sortedRegions(grouped)

	// Verify geographic order: 北京 < 广东 < 四川 < 香港 < UPOS < Gotcha < 其他
	indexOf := func(name string) int {
		for i, r := range regions {
			if r == name {
				return i
			}
		}
		return -1
	}

	pairs := [][2]string{
		{"北京", "广东-广州"},
		{"广东-广州", "四川-成都"},
		{"四川-成都", "香港"},
		{"香港", "UPOS-阿里云"},
		{"UPOS-阿里云", "Gotcha-海外"},
		{"Gotcha-海外", "其他"},
	}
	for _, p := range pairs {
		a, b := indexOf(p[0]), indexOf(p[1])
		if a >= b {
			t.Errorf("%q (idx %d) should come before %q (idx %d)", p[0], a, p[1], b)
		}
	}
}

func TestRegionMapCoversAllBaseLocations(t *testing.T) {
	for _, loc := range baseLocations {
		if _, ok := regionMap[loc]; !ok {
			t.Errorf("baseLocation %q missing from regionMap", loc)
		}
	}
}

func TestRenderJSON(t *testing.T) {
	grouped := map[string][]string{
		"北京": {"cn-bj-fx-01-04.bilivideo.com"},
	}
	data, err := renderJSON(grouped)
	if err != nil {
		t.Fatalf("renderJSON: %v", err)
	}
	// Should be valid JSON
	var parsed map[string][]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	if len(parsed["北京"]) != 1 {
		t.Errorf("expected 1 domain in 北京, got %d", len(parsed["北京"]))
	}
}

func TestRenderYAML(t *testing.T) {
	grouped := map[string][]string{
		"北京": {"cn-bj-fx-01-04.bilivideo.com", "cn-bj-fx-01-05.bilivideo.com"},
	}
	data := renderYAML(grouped)
	s := string(data)
	if !strings.Contains(s, "北京:") {
		t.Error("YAML missing region header")
	}
	if !strings.Contains(s, "  - cn-bj-fx-01-04.bilivideo.com") {
		t.Error("YAML missing domain entry")
	}
}

func TestRenderTXT(t *testing.T) {
	grouped := map[string][]string{
		"北京": {"cn-bj-fx-01-04.bilivideo.com"},
	}
	data := renderTXT(grouped)
	s := string(data)
	if !strings.Contains(s, "北京\n") {
		t.Error("TXT missing region header")
	}
	if !strings.Contains(s, "cn-bj-fx-01-04.bilivideo.com\n") {
		t.Error("TXT missing domain")
	}
}

func TestRenderMD(t *testing.T) {
	grouped := map[string][]string{
		"北京":       {"cn-bj-fx-01-04.bilivideo.com"},
		"UPOS-阿里云": {"upos-sz-mirroraliov.bilivideo.com"},
	}
	data := renderMD(grouped)
	s := string(data)
	if !strings.Contains(s, "# BiliCDN 节点列表") {
		t.Error("MD missing title")
	}
	if !strings.Contains(s, "## 🏙️ 直辖市") {
		t.Error("MD missing area group")
	}
	if !strings.Contains(s, "### 北京") {
		t.Error("MD missing region header")
	}
	if !strings.Contains(s, "| --- | --- |") {
		t.Error("MD missing table separator")
	}
	// Should not end with multiple newlines
	if strings.HasSuffix(s, "\n\n") {
		t.Error("MD ends with multiple newlines (MD012)")
	}
}

func TestRunConvertJSON(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "domains.txt")
	outputPath := filepath.Join(dir, "nodes.json")

	input := "cn-bj-fx-01-04.bilivideo.com\nupos-sz-mirroraliov.bilivideo.com\n"
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if err := runConvert([]string{"-i", inputPath, "-o", outputPath}); err != nil {
		t.Fatalf("runConvert: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var parsed map[string][]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 regions, got %d", len(parsed))
	}
}

func TestRunConvertFormatDetection(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "domains.txt")
	if err := os.WriteFile(inputPath, []byte("cn-bj-fx-01-04.bilivideo.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	extensions := []struct {
		ext    string
		marker string
	}{
		{".json", "{"},
		{".yml", "北京:"},
		{".yaml", "北京:"},
		{".txt", "北京\n"},
		{".md", "# BiliCDN"},
	}

	for _, tt := range extensions {
		outPath := filepath.Join(dir, "out"+tt.ext)
		if err := runConvert([]string{"-i", inputPath, "-o", outPath}); err != nil {
			t.Errorf("runConvert %s: %v", tt.ext, err)
			continue
		}
		data, _ := os.ReadFile(outPath)
		if !strings.Contains(string(data), tt.marker) {
			t.Errorf("%s: missing marker %q", tt.ext, tt.marker)
		}
	}
}

func TestRunConvertForceFormat(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "domains.txt")
	outputPath := filepath.Join(dir, "output") // no extension

	if err := os.WriteFile(inputPath, []byte("cn-bj-fx-01-04.bilivideo.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runConvert([]string{"-i", inputPath, "-o", outputPath, "-f", "yaml"}); err != nil {
		t.Fatalf("runConvert: %v", err)
	}
	data, _ := os.ReadFile(outputPath)
	if !strings.Contains(string(data), "北京:") {
		t.Error("forced yaml format not applied")
	}
}

func TestRunConvertUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "domains.txt")
	if err := os.WriteFile(inputPath, []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runConvert([]string{"-i", inputPath, "-o", "out", "-f", "xml"})
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("expected unknown format error, got: %v", err)
	}
}

func TestRegionSortKeyUnknown(t *testing.T) {
	key := regionSortKey("火星基地")
	if !strings.HasPrefix(key, "999-") {
		t.Errorf("unknown region sort key = %q, want 999- prefix", key)
	}
}

func TestMdTable(t *testing.T) {
	var b strings.Builder
	mdTable(&b, []string{"A", "B"}, [][]string{
		{"1", "hello"},
		{"2", "world"},
	})
	s := b.String()
	if !strings.Contains(s, "| A | B |") {
		t.Error("missing header")
	}
	if !strings.Contains(s, "| --- | --- |") {
		t.Error("missing separator")
	}
	if !strings.Contains(s, "| 1 | hello |") {
		t.Error("missing row")
	}
}

func TestRunConvertMissingInput(t *testing.T) {
	err := runConvert([]string{"-i", "/nonexistent/file.txt", "-o", "/tmp/out.json"})
	if err == nil {
		t.Fatal("expected error for missing input")
	}
}

func TestRunConvertEmptyInput(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "empty.txt")
	outputPath := filepath.Join(dir, "out.json")
	if err := os.WriteFile(inputPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runConvert([]string{"-i", inputPath, "-o", outputPath}); err != nil {
		t.Fatalf("runConvert empty: %v", err)
	}
	data, _ := os.ReadFile(outputPath)
	if string(data) != "{\n}\n" {
		t.Errorf("empty input should produce empty JSON, got: %q", data)
	}
}

func TestRunConvertSkipsBlankLines(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "domains.txt")
	outputPath := filepath.Join(dir, "out.json")

	input := "\n\ncn-bj-fx-01-04.bilivideo.com\n\n  \n"
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runConvert([]string{"-i", inputPath, "-o", outputPath}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(outputPath)
	var parsed map[string][]string
	json.Unmarshal(data, &parsed)

	total := 0
	for _, domains := range parsed {
		total += len(domains)
	}
	if total != 1 {
		t.Errorf("expected 1 domain (blank lines skipped), got %d", total)
	}
}
