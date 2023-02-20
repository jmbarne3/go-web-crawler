package crawler

import "time"

type AnalyticRecord struct {
	Domain         string
	PageViews      int
	UniqueViews    int
	AvgTimePage    time.Duration
	BounceRate     float32
	ExitPercentage float32
}
