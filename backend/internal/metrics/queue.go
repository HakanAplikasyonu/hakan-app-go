package metrics

import (
	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
)

// QueueInspector is the asynq.Inspector subset the collector needs.
type QueueInspector interface {
	GetQueueInfo(queue string) (*asynq.QueueInfo, error)
}

// QueueCollector reports the depth of background queues on every scrape.
type QueueCollector struct {
	insp   QueueInspector
	queues []string
	size   *prometheus.Desc
	lat    *prometheus.Desc
	up     *prometheus.Desc
}

// NewQueueCollector reports queues through insp.
func NewQueueCollector(insp QueueInspector, queues []string) *QueueCollector {
	return &QueueCollector{
		insp: insp, queues: queues,
		size: prometheus.NewDesc("queue_tasks", "Background tasks in a queue by state.", []string{"queue", "state"}, nil),
		lat:  prometheus.NewDesc("queue_latency_seconds", "Age of the oldest pending task in a queue.", []string{"queue"}, nil),
		up:   prometheus.NewDesc("queue_inspect_up", "1 when the queue could be inspected.", []string{"queue"}, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *QueueCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.size
	ch <- c.lat
	ch <- c.up
}

// Collect implements prometheus.Collector.
func (c *QueueCollector) Collect(ch chan<- prometheus.Metric) {
	for _, q := range c.queues {
		info, err := c.insp.GetQueueInfo(q)
		if err != nil {
			// A queue that has never had a task does not exist yet.
			ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0, q)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1, q)
		for state, n := range map[string]int{
			"pending": info.Pending, "active": info.Active, "scheduled": info.Scheduled,
			"retry": info.Retry, "archived": info.Archived,
		} {
			ch <- prometheus.MustNewConstMetric(c.size, prometheus.GaugeValue, float64(n), q, state)
		}
		ch <- prometheus.MustNewConstMetric(c.lat, prometheus.GaugeValue, info.Latency.Seconds(), q)
	}
}
