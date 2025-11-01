package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// defaultPattern 是 Fresh HTTP Proxy 列表的分页模板，%d 代表页码。
	defaultPattern = "https://list.proxylistplus.com/Fresh-HTTP-Proxy-List-%d"
)

var (
	// proxyRowRe 捕获代理数据所在的每一行。
	proxyRowRe = regexp.MustCompile(`(?is)<tr[^>]*class\s*=\s*['\"]?cells2?['\"]?[^>]*>.*?</tr>`)
	// cellRe 抽取单元格内的原始 HTML 内容。
	cellRe = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	// tagRe 用于从单元格文本中移除残留的标签。
	tagRe = regexp.MustCompile(`(?is)<[^>]+>`)
	// digitRe 用于解析端口号中的数字。
	digitRe = regexp.MustCompile(`\d+`)
)

// Proxy 表示解析得到的一条代理记录。
type Proxy struct {
	IP         string `json:"ip"`
	Port       string `json:"port"`
	Type       string `json:"type,omitempty"`
	Country    string `json:"country,omitempty"`
	Google     string `json:"google,omitempty"`
	HTTPS      string `json:"https,omitempty"`
	SourcePage int    `json:"page"`
}

func main() {
	outputDir := flag.String("output", "output", "保存抓取结果的根目录")
	pageCount := flag.Int("pages", 6, "需要抓取的页数")
	timeout := flag.Duration("timeout", 20*time.Second, "HTTP 请求超时时间")
	pattern := flag.String("pattern", defaultPattern, "分页 URL 模板（必须包含一个 %d 占位符）")
	flag.Parse()

	if *pageCount <= 0 {
		log.Fatalf("pages 必须大于 0")
	}
	if !strings.Contains(*pattern, "%d") {
		log.Fatalf("pattern 必须包含 %%d 以替换页码")
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	htmlDir := filepath.Join(*outputDir, "html")
	proxiesDir := filepath.Join(*outputDir, "proxies")
	for _, dir := range []string{htmlDir, proxiesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("创建子目录失败: %v", err)
		}
	}

	client := &http.Client{Timeout: *timeout}
	var allProxies []Proxy

	for page := 1; page <= *pageCount; page++ {
		pageURL := fmt.Sprintf(*pattern, page)
		body, err := downloadPage(client, pageURL)
		if err != nil {
			log.Fatalf("抓取第 %d 页失败: %v", page, err)
		}

		htmlPath := filepath.Join(htmlDir, fmt.Sprintf("page-%d.html", page))
		if err := os.WriteFile(htmlPath, body, 0o644); err != nil {
			log.Fatalf("保存第 %d 页 HTML 失败: %v", page, err)
		}

		proxies, err := parseFreshProxyPage(string(body))
		if err != nil {
			log.Fatalf("解析第 %d 页代理失败: %v", page, err)
		}
		for i := range proxies {
			proxies[i].SourcePage = page
		}

		pageListPath := filepath.Join(proxiesDir, fmt.Sprintf("page-%d.txt", page))
		if err := saveProxyText(pageListPath, proxies); err != nil {
			log.Fatalf("保存第 %d 页代理列表失败: %v", page, err)
		}

		allProxies = append(allProxies, proxies...)
		log.Printf("第 %d 页解析到 %d 条代理", page, len(proxies))
	}

	if len(allProxies) == 0 {
		log.Fatal("未解析到任何代理数据")
	}

	summaryTextPath := filepath.Join(*outputDir, "proxies.txt")
	if err := saveProxyText(summaryTextPath, allProxies); err != nil {
		log.Fatalf("保存汇总代理文本失败: %v", err)
	}

	summaryJSONPath := filepath.Join(*outputDir, "proxies.json")
	if err := saveProxyJSON(summaryJSONPath, allProxies); err != nil {
		log.Fatalf("保存汇总代理 JSON 失败: %v", err)
	}

	log.Printf("共保存 %d 条代理，输出目录：%s", len(allProxies), *outputDir)
}

// downloadPage 负责发起 HTTP 请求并返回页面内容。
func downloadPage(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ProxyCrawler/2.0; +https://example.com)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("响应状态码异常: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// parseFreshProxyPage 解析 Fresh HTTP Proxy 页面中的代理行。
func parseFreshProxyPage(doc string) ([]Proxy, error) {
	rows := proxyRowRe.FindAllString(doc, -1)
	if len(rows) == 0 {
		return nil, errors.New("页面中未找到代理数据行")
	}

	var proxies []Proxy
	for _, rowHTML := range rows {
		cells := cellRe.FindAllStringSubmatch(rowHTML, -1)
		if len(cells) == 0 {
			continue
		}

		values := make([]string, 0, len(cells))
		for _, c := range cells {
			values = append(values, sanitizeText(c[1]))
		}

		ipIdx := findIPIndex(values)
		if ipIdx == -1 || ipIdx+1 >= len(values) {
			continue
		}

		ip := values[ipIdx]
		port := normalizePort(values[ipIdx+1])
		if ip == "" || port == "" {
			continue
		}

		proxy := Proxy{IP: ip, Port: port}
		if ipIdx+2 < len(values) {
			proxy.Type = values[ipIdx+2]
		}
		if ipIdx+3 < len(values) {
			proxy.Country = values[ipIdx+3]
		}
		if ipIdx+4 < len(values) {
			proxy.Google = values[ipIdx+4]
		}
		if ipIdx+5 < len(values) {
			proxy.HTTPS = values[ipIdx+5]
		}

		proxies = append(proxies, proxy)
	}

	if len(proxies) == 0 {
		return nil, errors.New("未能从页面提取任何有效的代理 IP")
	}

	return proxies, nil
}

// findIPIndex 返回切片中首个合法 IP 地址的下标。
func findIPIndex(values []string) int {
	for idx, value := range values {
		if net.ParseIP(value) != nil {
			return idx
		}
	}
	return -1
}

// normalizePort 提取并验证端口号。
func normalizePort(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	digits := digitRe.FindString(trimmed)
	if digits == "" {
		return ""
	}
	port, err := strconv.Atoi(digits)
	if err != nil || port <= 0 || port > 65535 {
		return ""
	}
	return digits
}

// sanitizeText 去除标签、解码实体并压缩空白字符。
func sanitizeText(input string) string {
	if input == "" {
		return ""
	}
	noTags := tagRe.ReplaceAllString(input, "")
	decoded := html.UnescapeString(noTags)
	safe := stripInvalidUTF8(decoded)
	fields := strings.Fields(safe)
	return strings.Join(fields, " ")
}

// stripInvalidUTF8 去除非法的 UTF-8 字符，避免写文件时报错。
func stripInvalidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	buf := make([]rune, 0, len(s))
	for _, r := range s {
		if r == utf8.RuneError {
			continue
		}
		buf = append(buf, r)
	}
	return string(buf)
}

// saveProxyText 以 ip:port 形式写入文本文件。
func saveProxyText(path string, proxies []Proxy) error {
	var builder strings.Builder
	for idx, proxy := range proxies {
		if idx > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(proxy.IP)
		builder.WriteByte(':')
		builder.WriteString(proxy.Port)
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

// saveProxyJSON 以 JSON 格式写入代理数组。
func saveProxyJSON(path string, proxies []Proxy) error {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(proxies); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
