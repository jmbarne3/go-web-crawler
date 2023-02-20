package crawler

import (
	"crypto/tls"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/solywsh/chatgpt"
)

func Crawl(config *CrawlerConfig) {
	var domains []string
	var results []CrawlerResult

	parseDomainFile(&config.DomainFilePath, &domains)
	crawlDomains(config, &domains, &results)
	addAnalytics(config, results)
	writeOutput(config, results)
}

func parseDomainFile(filepath *string, domains *[]string) {
	f, err := os.Open(*filepath)

	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	reader := csv.NewReader(f)
	data, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	for i, line := range data {
		if i > 0 { // Skip the header line
			*domains = append(*domains, line[0])
		}
	}
}

func noRedirect(req *http.Request, via []*http.Request) error {
	return errors.New("don't redirect")
}

func crawlDomains(config *CrawlerConfig, domains *[]string, results *[]CrawlerResult) {
	var wg sync.WaitGroup

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	redirectClient := http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: noRedirect,
	}

	for _, domain := range *domains {
		result := CrawlerResult{
			Domain: domain,
		}

		http_url := fmt.Sprintf("http://%s/", domain)
		https_url := fmt.Sprintf("https://%s/", domain)

		wg.Add(4)

		go checkUrl(http_url, &client, &result.AnswersHttp, &wg)
		go checkRedirect(http_url, &redirectClient, &result.RedirectHttps, &wg)
		go checkUrl(https_url, &client, &result.AnswersHttps, &wg)
		go checkCertificate(domain, &result.ValidCertificate, &wg)

		wg.Wait()
		*results = append(*results, result)
	}

	var gptWg sync.WaitGroup
	chat := chatgpt.New(config.APIKey, "user_id(not required)", 30*time.Second)
	defer chat.Close()

	for _, result := range *results {
		if result.AnswersHttps {
			gptWg.Add(1)
			https_url := fmt.Sprintf("https://%s", result.Domain)
			go parsePage(https_url, &client, chat, &result, &gptWg)
		}
	}

	gptWg.Wait()
}

func checkUrl(url string, client *http.Client, respond *bool, wg *sync.WaitGroup) {
	resp, err := client.Get(url)

	if err != nil {
		*respond = false
		wg.Done()
		return
	}

	if (resp.StatusCode >= 200) && (resp.StatusCode < 300) {
		*respond = true
		wg.Done()
		return
	}

	*respond = false
	wg.Done()
}

func checkRedirect(url string, client *http.Client, redirect *bool, wg *sync.WaitGroup) {
	resp, err := client.Get(url)

	if strings.HasSuffix(err.Error(), ": no such host") {
		*redirect = false
		wg.Done()
		return
	}

	if (resp.StatusCode >= 301) && (resp.StatusCode <= 302) || (resp.StatusCode >= 307) && (resp.StatusCode <= 308) {
		*redirect = true
		wg.Done()
		return
	}

	*redirect = false
	wg.Done()
}

func checkCertificate(domain string, validCertificate *bool, wg *sync.WaitGroup) {
	domainWithPort := fmt.Sprintf("%s:%d", domain, 443)

	_, err := tls.Dial("tcp", domainWithPort, nil)

	if err != nil {
		*validCertificate = false
	} else {
		*validCertificate = true
	}

	wg.Done()
}

func parsePage(url string, client *http.Client, chat *chatgpt.ChatGPT, result *CrawlerResult, wg *sync.WaitGroup) {
	resp, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)

	if err == nil {
		title := doc.Find("title").First()
		if title != nil {
			result.Title = title.Text()
		}

		h1 := doc.Find("h1").First()
		if h1 != nil {
			result.H1Text = h1.Text()
		}

		doc.Find("h2").Each(func(i int, obj *goquery.Selection) {
			result.H2Text = append(result.H2Text, obj.Text())
		})

		prompt := fmt.Sprintf("Summarize the content on the following page: %s", url)

		ans, err := chat.Chat(prompt)

		if err != nil {
			log.Fatal(err)
		}

		result.Description = ans
	}

	wg.Done()
}

func addAnalytics(config *CrawlerConfig, results []CrawlerResult) {
	records := make(map[string]AnalyticRecord)

	f, err := os.Open(config.AnalyticsFilePath)

	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	reader := csv.NewReader(f)
	data, err := reader.ReadAll()

	if err != nil {
		log.Fatal(err)
	}

	durationRegex := regexp.MustCompile(`(\d{2}):(\d{2}):(\d{2})`)

	for i, line := range data {
		if i > 0 { // Skip the header line
			domain := strings.TrimRight(line[0], "/")
			pageViews, _ := strconv.Atoi(strings.Replace(line[1], ",", "", -1))
			uniqueViews, _ := strconv.Atoi(strings.Replace(line[2], ",", "", -1))
			avgTimePage, _ := time.ParseDuration(durationRegex.ReplaceAllString(line[3], "${1}h${2}m${3}s"))
			bounceRate, _ := strconv.ParseFloat(strings.Replace(line[4], "%", "", 1), 32)
			exitPercentage, _ := strconv.ParseFloat(strings.Replace(line[5], "%", "", 1), 32)

			records[domain] = AnalyticRecord{
				Domain:         domain,
				PageViews:      pageViews,
				UniqueViews:    uniqueViews,
				AvgTimePage:    avgTimePage,
				BounceRate:     float32(bounceRate),
				ExitPercentage: float32(exitPercentage),
			}
		}
	}

	for _, result := range results {
		if val, ok := records[result.Domain]; ok {
			result.PageViews = val.PageViews
			result.UniqueViews = val.UniqueViews
			result.AvgTimePage = val.AvgTimePage
			result.BounceRate = val.BounceRate
			result.ExitPercentage = val.ExitPercentage
		}
	}
}

func writeOutput(config *CrawlerConfig, results []CrawlerResult) {
	f, err := os.Create(config.OutputFilePath)
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	writer := csv.NewWriter(f)
	if err != nil {
		log.Fatal(err)
	}

	defer writer.Flush()

	// Write the header line
	writer.Write([]string{
		"domain",
		"answer_http",
		"redirects_to_https",
		"answers_https",
		"valid_certificate",
		"title",
		"h1_text",
		"h2_text",
		"description",
		"page_views",
		"unique_page_views",
		"avg_time_page",
		"bounce_rate",
		"exit_percentage",
	})

	for _, result := range results {
		writer.Write([]string{
			result.Domain,
			fmt.Sprintf("%t", result.AnswersHttp),
			fmt.Sprintf("%t", result.RedirectHttps),
			fmt.Sprintf("%t", result.AnswersHttps),
			fmt.Sprintf("%t", result.ValidCertificate),
			result.Title,
			result.H1Text,
			strings.Join(result.H2Text, ", "),
			result.Description,
			fmt.Sprintf("%d", result.PageViews),
			fmt.Sprintf("%d", result.UniqueViews),
			fmt.Sprintf("%s", result.AvgTimePage),
			fmt.Sprintf("%f", result.BounceRate),
			fmt.Sprintf("%f", result.ExitPercentage),
		})
	}
}
