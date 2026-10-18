package telemetry

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

type prometheusBackend struct {
	options Options
	client  *http.Client
}

func newPrometheus(o Options) *prometheusBackend { return &prometheusBackend{o, telemetryClient()} }
func (b *prometheusBackend) Read(ctx context.Context, now time.Time) (*History, error) {
	h, err := fetch(ctx, b.client, b.options.PrometheusURL, b.options.Labels, now, b.options)
	if err == nil && h.sampleTime < now.Add(-b.options.Freshness).Unix() {
		return nil, errors.New("no fresh Prometheus samples")
	}
	return h, err
}
func (b *prometheusBackend) Write(ctx context.Context, p Point) error {
	if b.options.LegacyPrometheus {
		return nil
	}
	address, e := endpoint(b.options.PrometheusWriteURL, "")
	if e != nil {
		return e
	}
	labels := b.options.Labels
	for k := range b.options.Labels {
		if !labelName.MatchString(k) || k == "__name__" {
			return errors.New("invalid telemetry label")
		}
	}
	values := map[string]float64{"factorio_ups": number(p.UPS), "factorio_target_ups": number(p.Target), "factorio_players": number(p.Players), "factorio_sample_timestamp_seconds": float64(p.Time), "factorio_rcon_up": 1, "factorio_paused": 0}
	if p.State == "disconnected" || p.State == "unavailable" {
		values["factorio_rcon_up"] = 0
	}
	if p.State == "paused" {
		values["factorio_paused"] = 1
	}
	request := prompb.WriteRequest{}
	for name, value := range values {
		series := prompb.TimeSeries{Labels: []prompb.Label{{Name: "__name__", Value: name}}, Samples: []prompb.Sample{{Value: value, Timestamp: p.Time * 1000}}}
		for k, v := range labels {
			series.Labels = append(series.Labels, prompb.Label{Name: k, Value: v})
		}
		sort.Slice(series.Labels, func(i, j int) bool { return series.Labels[i].Name < series.Labels[j].Name })
		request.Timeseries = append(request.Timeseries, series)
	}
	data, e := request.Marshal()
	if e != nil {
		return errors.New("cannot encode UPS metrics")
	}
	_, e = send(ctx, b.client, http.MethodPost, address, b.options.PrometheusToken, snappy.Encode(nil, data), map[string]string{"Content-Type": "application/x-protobuf", "Content-Encoding": "snappy", "X-Prometheus-Remote-Write-Version": "0.1.0", "User-Agent": "factorio-server-manager"})
	return e
}
func number(v *float64) float64 {
	if v == nil {
		return math.NaN()
	}
	return *v
}
