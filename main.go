package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Proxy 表示从代理表格中提取的一条记录。
// 字段全部使用字符串类型，方便应对站点上可能出现的混合格式
// （例如 "Google" 列可能是 "no" 或 "yes"，而不是布尔值）。
type Proxy struct {
	IP          string `json:"ip"`
	Port        string `json:"port"`
	Protocol    string `json:"protocol,omitempty"`
	Country     string `json:"country,omitempty"`
	Anonymity   string `json:"anonymity,omitempty"`
	Google      string `json:"google,omitempty"`
	HTTPS       string `json:"https,omitempty"`
	LastChecked string `json:"last_checked,omitempty"`
}

// ProxyPool 根据表格标题或生成的名称对代理进行分组。
type ProxyPool struct {
	Name    string  `json:"name"`
	Proxies []Proxy `json:"proxies"`
}

var (
        // tableRe 大致截取 HTML 中的每个表格区块，便于后续提取代理数据。
        tableRe = regexp.MustCompile(`(?is)<table[^>]*>.*?</table>`)
        // rowRe 从表格中抽取每一行 <tr>。
        rowRe = regexp.MustCompile(`(?is)<tr[^>]*>.*?</tr>`)
        // cellRe 抽取单元格 <td>/<th> 的文本内容。
        cellRe = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
        // captionRe 获取表格标题，用作代理池名称。
        captionRe = regexp.MustCompile(`(?is)<caption[^>]*>(.*?)</caption>`)
        // tagRe 在清洗单元格文本时去除残留的 HTML 标签。
        tagRe = regexp.MustCompile(`(?is)<[^>]+>`)
)

func main() {
        // 命令行参数可控制输出格式、目标 URL 以及 HTTP 超时时间。
	outputJSON := flag.Bool("json", false, "print result as JSON")
	url := flag.String("url", "https://list.proxylistplus.com/index.php", "page to scrape")
	timeout := flag.Duration("timeout", 15*time.Second, "HTTP request timeout")
	flag.Parse()

	pools, err := FetchProxyPools(*url, *timeout)
	if err != nil {
		log.Fatalf("failed to fetch proxies: %v", err)
	}

	if *outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(pools); err != nil {
			log.Fatalf("failed to encode JSON: %v", err)
		}
		return
	}

	for _, pool := range pools {
		fmt.Printf("=== %s ===\n", pool.Name)
		for _, proxy := range pool.Proxies {
			fmt.Printf("%s:%s", proxy.IP, proxy.Port)
			extra := buildExtra(proxy)
			if extra != "" {
				fmt.Printf("\t%s", extra)
			}
			fmt.Println()
		}
		fmt.Println()
	}
}

// buildExtra 将可选字段整理成便于阅读的字符串。
func buildExtra(p Proxy) string {
	var items []string
	add := func(label, value string) {
		if value == "" {
			return
		}
		items = append(items, fmt.Sprintf("%s=%s", label, value))
	}

	add("protocol", p.Protocol)
	add("country", p.Country)
	add("anonymity", p.Anonymity)
	add("google", p.Google)
	add("https", p.HTTPS)
	add("last_checked", p.LastChecked)

	if len(items) == 0 {
		return ""
	}

	sort.Strings(items)
	return strings.Join(items, ", ")
}

// FetchProxyPools 下载目标页面并从中解析代理池。
func FetchProxyPools(url string, timeout time.Duration) ([]ProxyPool, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ProxyCrawler/1.0; +https://example.com)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseProxyPools(string(body))
}

// parseProxyPools 找到页面中相关的代理表格并转换为结构化的 ProxyPool 列表。
func parseProxyPools(htmlDoc string) ([]ProxyPool, error) {
	tables := tableRe.FindAllStringIndex(htmlDoc, -1)
	if len(tables) == 0 {
		return nil, errors.New("no tables found in page")
	}

	var pools []ProxyPool
	for i, pos := range tables {
		tableHTML := htmlDoc[pos[0]:pos[1]]
		if !strings.Contains(strings.ToLower(tableHTML), "ip address") {
			continue
		}

		header, rows := extractRows(tableHTML)
		if len(header) == 0 || len(rows) == 0 {
			continue
		}

		proxies := make([]Proxy, 0, len(rows))
		for _, cells := range rows {
			proxy, ok := buildProxy(header, cells)
			if !ok {
				continue
			}
			proxies = append(proxies, proxy)
		}

		if len(proxies) == 0 {
			continue
		}

		poolName := extractTableTitle(tableHTML)
		if poolName == "" {
			poolName = fmt.Sprintf("Proxy Pool %d", len(pools)+1)
		}

		pools = append(pools, ProxyPool{Name: poolName, Proxies: proxies})
		_ = i
	}

	if len(pools) == 0 {
		return nil, errors.New("no proxy tables detected")
	}

	return pools, nil
}

// extractTableTitle 返回表格的标题（若存在）。
func extractTableTitle(tableHTML string) string {
	match := captionRe.FindStringSubmatch(tableHTML)
	if match == nil {
		return ""
	}
	return sanitizeText(match[1])
}

// extractRows 将表格拆分为表头和数据行。
func extractRows(tableHTML string) ([]string, [][]string) {
	matches := rowRe.FindAllString(tableHTML, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	headerCells := extractCells(matches[0])
	if len(headerCells) == 0 {
		return nil, nil
	}

	var data [][]string
	for _, row := range matches[1:] {
		cells := extractCells(row)
		if len(cells) == 0 {
			continue
		}
		data = append(data, cells)
	}

	return headerCells, data
}

// extractCells 清洗一行内容并返回单元格值。
func extractCells(rowHTML string) []string {
	matches := cellRe.FindAllStringSubmatch(rowHTML, -1)
	if len(matches) == 0 {
		return nil
	}

	cells := make([]string, 0, len(matches))
	for _, m := range matches {
		cleaned := sanitizeText(m[1])
		if cleaned == "" {
			continue
		}
		cells = append(cells, cleaned)
	}
	return cells
}

// sanitizeText 去除 HTML 标签并规范空白字符。
func sanitizeText(input string) string {
	withoutTags := tagRe.ReplaceAllString(input, "")
	trimmed := strings.TrimSpace(html.UnescapeString(withoutTags))
	return strings.Join(strings.Fields(trimmed), " ")
}

// buildProxy 根据表头映射单元格内容，构造 Proxy。
func buildProxy(headers, cells []string) (Proxy, bool) {
	headerIndex := make(map[string]int, len(headers))
	for idx, header := range headers {
		normalized := normalizeHeader(header)
		headerIndex[normalized] = idx
	}

	ipIdx, okIP := findIndex(headerIndex, "ipaddress", "ip", "proxyaddress")
	portIdx, okPort := findIndex(headerIndex, "port", "portnumber")

	if !okIP || !okPort {
		return Proxy{}, false
	}

	if ipIdx >= len(cells) || portIdx >= len(cells) {
		return Proxy{}, false
	}

	proxy := Proxy{IP: cells[ipIdx], Port: cells[portIdx]}

	if idx, ok := findIndex(headerIndex, "protocol", "type"); ok && idx < len(cells) {
		proxy.Protocol = cells[idx]
	}
	if idx, ok := findIndex(headerIndex, "country", "nation"); ok && idx < len(cells) {
		proxy.Country = cells[idx]
	}
	if idx, ok := findIndex(headerIndex, "anonymity", "anon", "level"); ok && idx < len(cells) {
		proxy.Anonymity = cells[idx]
	}
	if idx, ok := findIndex(headerIndex, "google"); ok && idx < len(cells) {
		proxy.Google = cells[idx]
	}
	if idx, ok := findIndex(headerIndex, "https", "ssl"); ok && idx < len(cells) {
		proxy.HTTPS = cells[idx]
	}
	if idx, ok := findIndex(headerIndex, "lastchecked", "lastupdate", "updated"); ok && idx < len(cells) {
		proxy.LastChecked = cells[idx]
	}

	return proxy, true
}

// normalizeHeader 扁平化表头字符串，容忍空格、标点、大小写等细微差异。
func normalizeHeader(header string) string {
	lowered := strings.ToLower(header)
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", ":", "", "#", "", "?", "")
	return replacer.Replace(lowered)
}

// findIndex 返回匹配给定键的首个下标。
func findIndex(index map[string]int, keys ...string) (int, bool) {
	for _, key := range keys {
		if idx, ok := index[key]; ok {
			return idx, true
		}
	}
	return 0, false
}
