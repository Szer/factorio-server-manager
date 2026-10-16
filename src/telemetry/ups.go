// Package telemetry reads a bounded UPS history from a configured Prometheus server.
package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Point struct {
	Time    int64    `json:"time"`
	UPS     *float64 `json:"ups"`
	Target  *float64 `json:"target"`
	Players *float64 `json:"players"`
	State   string   `json:"state"`
}
type History struct {
	Configured bool    `json:"configured"`
	Start      int64   `json:"start"`
	End        int64   `json:"end"`
	Points     []Point `json:"points"`
}
type Series struct {
	Metric map[string]string    `json:"metric"`
	Values [][2]json.RawMessage `json:"values"`
}

var labelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

const metricNames = "factorio_ups|factorio_target_ups|factorio_players|factorio_paused|factorio_rcon_up|factorio_sample_timestamp_seconds"

func Fetch(ctx context.Context, client *http.Client, base string, labels map[string]string, now time.Time) (*History, error) {
	end := now.Unix() / 10 * 10
	history := &History{Configured: base != "", Start: end - 300, End: end, Points: []Point{}}
	if base == "" {
		return history, nil
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("invalid telemetry URL")
	}
	keys := []string{}
	for key := range labels {
		if !labelName.MatchString(key) || key == "__name__" {
			return nil, errors.New("invalid telemetry label")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	selector := `{__name__=~"` + metricNames + `"`
	for _, key := range keys {
		selector += "," + key + "=" + strconv.Quote(labels[key])
	}
	selector += "}"
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/query_range"
	query := url.Values{"query": {selector}, "start": {strconv.FormatInt(history.Start, 10)}, "end": {strconv.FormatInt(end, 10)}, "step": {"10s"}, "lookback_delta": {"20s"}, "timeout": {"4s"}}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("telemetry request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telemetry returned HTTP %d", response.StatusCode)
	}
	var envelope struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string   `json:"resultType"`
			Result     []Series `json:"result"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope); err != nil {
		return nil, errors.New("invalid telemetry response")
	}
	if envelope.Status != "success" || envelope.Data.ResultType != "matrix" {
		return nil, errors.New("telemetry query failed")
	}
	history.Points, err = normalize(envelope.Data.Result, history.Start, end)
	return history, err
}
func normalize(series []Series, start, end int64) ([]Point, error) {
	metrics := map[string]map[int64]float64{}
	for _, s := range series {
		name := s.Metric["__name__"]
		if !strings.Contains("|"+metricNames+"|", "|"+name+"|") {
			continue
		}
		if metrics[name] == nil {
			metrics[name] = map[int64]float64{}
		}
		for _, pair := range s.Values {
			var timestamp float64
			var text string
			if json.Unmarshal(pair[0], &timestamp) != nil || json.Unmarshal(pair[1], &text) != nil {
				return nil, errors.New("invalid metric sample")
			}
			value, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}
			if timestamp < float64(start) || timestamp > float64(end) || timestamp != math.Trunc(timestamp) {
				continue
			}
			t := int64(timestamp)
			if (t-start)%10 != 0 {
				continue
			}
			if _, exists := metrics[name][t]; exists {
				return nil, errors.New("multiple exporters matched; narrow telemetry labels")
			}
			metrics[name][t] = value
		}
	}
	get := func(name string, t int64) *float64 {
		if v, ok := metrics[name][t]; ok {
			return &v
		}
		return nil
	}
	points := []Point{}
	for t := start; t <= end; t += 10 {
		p := Point{Time: t, State: "unavailable"}
		up := get("factorio_rcon_up", t)
		sample := get("factorio_sample_timestamp_seconds", t)
		if up != nil && *up == 0 {
			p.State = "disconnected"
		} else if up != nil && *up == 1 && sample != nil && float64(t)-*sample >= -5 && float64(t)-*sample <= 30 {
			p.Target = get("factorio_target_ups", t)
			p.Players = get("factorio_players", t)
			paused := get("factorio_paused", t)
			if paused != nil && *paused == 1 {
				p.State = "paused"
			} else if paused != nil && *paused == 0 {
				p.UPS = get("factorio_ups", t)
				p.State = "collecting"
				if p.UPS != nil && *p.UPS >= 0 {
					p.State = "active"
				} else {
					p.UPS = nil
				}
			}
		}
		points = append(points, p)
	}
	return points, nil
}
