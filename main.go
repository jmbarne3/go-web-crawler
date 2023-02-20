package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jmbarne3/web-crawler/crawler"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	var domain_path string
	var analytics_path string
	var output_path string

	api_key := flag.String("api-key", os.Getenv("OPEN_AI_API_KEY"), "The API key to use to connect to the Open AI API.")

	flag.Parse()

	for idx, val := range flag.Args() {
		switch idx {
		case 0:
			domain_path = val
		case 1:
			analytics_path = val
		case 2:
			output_path = val
		}
	}

	if (domain_path == "") || (analytics_path == "") || (output_path == "") {
		fmt.Println(usage())
		os.Exit(1)
	}

	if *api_key == "" {
		fmt.Println("An API Key is required. Please add it to your .env file or provide is as an argument.")
		fmt.Println(usage())
		os.Exit(1)
	}

	config := crawler.CrawlerConfig{
		DomainFilePath:    domain_path,
		AnalyticsFilePath: analytics_path,
		OutputFilePath:    output_path,
	}

	crawler.Crawl(&config)
}

func usage() string {
	s := "go-web-crawler [domain-file-path] [analytics-file-path] [output-file-path] [--api-key=<api-key>]"
	return s
}
