package telemetry

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Backends                                                             []string
	Interval, ProbeTimeout, ReadTimeout, WriteTimeout, ReadBudget        time.Duration
	Window, Freshness, Cooldown, LogInterval, Retention, CleanupInterval time.Duration
	Failures, QueueSize                                                  int
	PrometheusURL, PrometheusWriteURL, PrometheusToken                   string
	Labels                                                               map[string]string
	BeszelURL, BeszelToken, BeszelCollection, ServerID                   string
	LegacyPrometheus                                                     bool
}

func LoadOptions(legacyURL string, labels map[string]string) Options {
	o := Options{Labels: labels, PrometheusURL: env("UPS_PROMETHEUS_URL", legacyURL), PrometheusToken: os.Getenv("UPS_PROMETHEUS_TOKEN"),
		BeszelURL: os.Getenv("UPS_BESZEL_URL"), BeszelToken: os.Getenv("UPS_BESZEL_TOKEN"), BeszelCollection: env("UPS_BESZEL_COLLECTION", "factorio_ups"), ServerID: env("UPS_SERVER_ID", "factorio")}
	o.Interval = duration("UPS_SAMPLE_INTERVAL", 10*time.Second, time.Second, time.Minute)
	o.ProbeTimeout = duration("UPS_PROBE_TIMEOUT", 3*time.Second, time.Millisecond, time.Minute)
	o.ReadTimeout = duration("UPS_READ_TIMEOUT", 3*time.Second, time.Millisecond, time.Minute)
	o.WriteTimeout = duration("UPS_WRITE_TIMEOUT", 3*time.Second, time.Millisecond, time.Minute)
	o.ReadBudget = duration("UPS_READ_TOTAL_TIMEOUT", 10*time.Second, time.Millisecond, 3*time.Minute)
	o.Window = duration("UPS_HISTORY_WINDOW", 5*time.Minute, o.Interval, 24*time.Hour)
	o.Freshness = duration("UPS_FRESHNESS", 3*o.Interval, o.Interval, time.Hour)
	o.Cooldown = duration("UPS_CIRCUIT_COOLDOWN", time.Minute, time.Millisecond, time.Hour)
	o.LogInterval = duration("UPS_LOG_INTERVAL", 5*time.Minute, time.Second, 24*time.Hour)
	o.Retention = duration("UPS_BESZEL_RETENTION", 24*time.Hour, o.Window, 365*24*time.Hour)
	o.CleanupInterval = duration("UPS_BESZEL_CLEANUP_INTERVAL", time.Minute, time.Second, 24*time.Hour)
	o.Failures = integer("UPS_CIRCUIT_FAILURES", 3, 1, 100)
	o.QueueSize = integer("UPS_WRITE_QUEUE_SIZE", 6, 1, 10000)
	o.PrometheusWriteURL = env("UPS_PROMETHEUS_WRITE_URL", strings.TrimRight(o.PrometheusURL, "/")+"/api/v1/write")
	if v := os.Getenv("UPS_PROMETHEUS_LABELS"); v != "" {
		var parsed map[string]string
		if json.Unmarshal([]byte(v), &parsed) == nil {
			o.Labels = parsed
		} else {
			log.Print("UPS: invalid UPS_PROMETHEUS_LABELS; keeping configured labels")
		}
	}
	raw, explicit := os.LookupEnv("UPS_BACKENDS")
	if !explicit {
		raw = "local"
		if o.PrometheusURL != "" {
			raw = "prometheus"
			o.LegacyPrometheus = true
		}
	}
	seen := map[string]bool{}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name != "local" && name != "prometheus" && name != "beszel" {
			log.Print("UPS: ignoring unsupported backend in UPS_BACKENDS")
			continue
		}
		if !seen[name] {
			o.Backends = append(o.Backends, name)
			seen[name] = true
		}
	}
	if len(o.Backends) == 0 {
		log.Print("UPS: no valid backends; using local")
		o.Backends = []string{"local"}
	}

	if seen["prometheus"] && !o.LegacyPrometheus {
		copied := make(map[string]string, len(o.Labels)+1)
		for key, value := range o.Labels {
			copied[key] = value
		}
		if _, ok := copied["server"]; !ok {
			copied["server"] = o.ServerID
		}
		o.Labels = copied
	}
	return o
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func duration(key string, fallback, min, max time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, e := time.ParseDuration(v)
		if e == nil && d >= min && d <= max && (key != "UPS_SAMPLE_INTERVAL" || d%time.Second == 0) {
			return d
		}
		log.Printf("UPS: invalid %s; using %s", key, fallback)
	}
	return fallback
}
func integer(key string, fallback, min, max int) int {
	if v := os.Getenv(key); v != "" {
		n, e := strconv.Atoi(v)
		if e == nil && n >= min && n <= max {
			return n
		}
		log.Printf("UPS: invalid %s; using %d", key, fallback)
	}
	return fallback
}
