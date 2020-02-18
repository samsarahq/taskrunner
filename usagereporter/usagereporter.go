package usagereporter

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/samsarahq/taskrunner"
	"github.com/samsarahq/taskrunner/clireporter"
	"github.com/zorkian/go-datadog-api"
)

func Option(r *taskrunner.Runtime) {
	var apiKey string
	var datadogClient *datadog.Client

	stdout := &clireporter.PrefixedWriter{Writer: os.Stdout, Prefix: "usagereporter", Separator: "👀"}
	stderr := &clireporter.PrefixedWriter{Writer: os.Stderr, Prefix: "usagereporter", Separator: "⚠️"}

	apiKey = os.Getenv("DATADOG_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(stderr, "Missing env var DATADOG_API_KEY! Usage monitoring disabled.")
		return
	}
	teamName := os.Getenv("TEAM_NAME")
	if teamName == "" {
		teamName = "Unassigned"
		fmt.Fprintf(stdout, "Missing env var TEAM_NAME, using '%s' as a team name instead", teamName)
	}
	fmt.Fprintf(stdout, "Taskrunner usage monitoring started.")
	datadogClient = datadog.NewClient(apiKey, apiKey)

	r.OnStart(func(ctx context.Context, executor *taskrunner.Executor) error {
		fmt.Fprintf(stdout, "Taskrunner usage monitoring starting...")
		return nil
	})

	r.Subscribe(func(events <-chan taskrunner.ExecutorEvent) error {
		for event := range events {
			var task *taskrunner.Task
			if handler := event.TaskHandler(); handler != nil {
				task = handler.Definition()
			}

			tags := []string{
				fmt.Sprintf("task:%s", task.Name),
				fmt.Sprintf("team:%s", teamName),
			}
			switch event := event.(type) {
			case *taskrunner.TaskLogEvent:
				datadogClient.PostMetrics([]datadog.Metric{counterMetric(time.Now(), "taskrunner.usage.log", 1, tags, 1)})
				logSize := float64(len(event.Message))
				// If a log is very long it is either just junk (and should be removed) or is probably a stacktrace
				datadogClient.PostMetrics([]datadog.Metric{gaugeMetric(time.Now(), "taskrunner.usage.logsize", logSize, tags)})
			case *taskrunner.TaskStartedEvent:
				datadogClient.PostMetrics([]datadog.Metric{counterMetric(time.Now(), "taskrunner.usage.start", 1, tags, 1)})
			case *taskrunner.TaskCompletedEvent:
				durationMs := float64(event.Duration) / float64(time.Second)
				datadogClient.PostMetrics([]datadog.Metric{gaugeMetric(time.Now(), "taskrunner.usage.duration", durationMs, tags)})
			case *taskrunner.TaskFailedEvent:
				datadogClient.PostMetrics([]datadog.Metric{counterMetric(time.Now(), "taskrunner.usage.failed", 1, tags, 1)})
			}
		}

		return nil
	})
}

// buildMetric builds a datadog.Metric object.
func buildMetric(timestamp time.Time, name string, value float64, tags []string, metricType string) datadog.Metric {
	return datadog.Metric{
		Metric: &name,
		Points: []datadog.DataPoint{
			{
				float64(timestamp.Unix()),
				value,
			},
		},
		Type: &metricType,
		Tags: tags,
	}
}

// counterMetric creates a new Counter metric for the DatadogHTTP Client.
func counterMetric(timestamp time.Time, name string, value float64, tags []string, rate float64) datadog.Metric {
	return buildMetric(timestamp, name, value/rate, tags, "count")
}

// gaugeMetric creates a new Gauge metric for the DatadogHTTP Client.
func gaugeMetric(timestamp time.Time, name string, value float64, tags []string) datadog.Metric {
	return buildMetric(timestamp, name, value, tags, "gauge")
}
