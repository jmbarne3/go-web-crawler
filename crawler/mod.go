package crawler

import (
	"context"
	"crypto/tls"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/PullRequestInc/go-gpt3"
)

func Crawl(config *CrawlerConfig) {
	var domains []string
	var results []CrawlerResult

	parseDomainFile(&config.DomainFilePath, &domains)
	crawlDomains(config, &domains, &results)
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
	ctx := context.Background()
	gptClient := gpt3.NewClient(config.APIKey)

	for _, result := range *results {
		if result.AnswersHttps {
			gptWg.Add(1)
			https_url := fmt.Sprintf("https://%s", result.Domain)
			go parsePage(https_url, &client, &ctx, gptClient, &result, &gptWg)
		}
	}

	gptWg.Wait()

	for _, result := range *results {
		fmt.Println(result)
		// fmt.Println(result.Description)
	}
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
	resp, _ := client.Get(url)

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

func parsePage(url string, client *http.Client, ctx *context.Context, gptClient gpt3.Client, result *CrawlerResult, wg *sync.WaitGroup) {
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

		prompt := fmt.Sprintf("Summarize the content on the home page of the website %s.", url)

		fmt.Println(prompt)

		resp, err := gptClient.Completion(*ctx, gpt3.CompletionRequest{
			Prompt:           []string{prompt},
			Temperature:      gpt3.Float32Ptr(0.7),
			MaxTokens:        gpt3.IntPtr(100),
			TopP:             gpt3.Float32Ptr(1.0),
			N:                gpt3.IntPtr(1),
			FrequencyPenalty: 0.0,
			PresencePenalty:  0.0,
			Echo:             false,
		})

		if err != nil {
			log.Fatal(err)
		}

		result.Description = resp.Choices[0].Text
		fmt.Println(result.Description)
	}

	wg.Done()
}
