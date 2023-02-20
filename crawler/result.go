package crawler

import "time"

type CrawlerResult struct {
	Domain           string
	AnswersHttp      bool
	RedirectHttps    bool
	AnswersHttps     bool
	ValidCertificate bool
	Title            string
	H1Text           string
	H2Text           []string
	Description      string
	PageViews        int
	UniqueViews      int
	AvgTimePage      time.Duration
	BounceRate       float32
	ExitPercentage   float32
}
