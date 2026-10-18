package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"time"
)

var collectionName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type beszelBackend struct {
	options     Options
	client      *http.Client
	nextCleanup time.Time
}
type beszelRecord struct {
	ID         string `json:"id,omitempty"`
	Server     string `json:"server"`
	SampleTime int64  `json:"sample_time"`
	Point      Point  `json:"point"`
}
type beszelPage struct {
	Items      []beszelRecord `json:"items"`
	TotalPages int            `json:"totalPages"`
}

func newBeszel(o Options) *beszelBackend {
	return &beszelBackend{options: o, client: telemetryClient()}
}
func (b *beszelBackend) address() (string, error) {
	if !collectionName.MatchString(b.options.BeszelCollection) {
		return "", errors.New("invalid Beszel collection")
	}
	if b.options.BeszelToken == "" {
		return "", errors.New("Beszel token missing")
	}
	return endpoint(b.options.BeszelURL, "/api/collections/"+b.options.BeszelCollection+"/records")
}
func (b *beszelBackend) filter() string { return "server = " + strconv.Quote(b.options.ServerID) }
func (b *beszelBackend) list(ctx context.Context, filter, order string, page, size int) (beszelPage, error) {
	var result beszelPage
	address, e := b.address()
	if e != nil {
		return result, e
	}
	q := url.Values{"filter": {filter}, "sort": {order}, "page": {strconv.Itoa(page)}, "perPage": {strconv.Itoa(size)}}
	data, e := send(ctx, b.client, http.MethodGet, address+"?"+q.Encode(), b.options.BeszelToken, nil, nil)
	if e != nil {
		return result, e
	}
	if json.Unmarshal(data, &result) != nil || result.Items == nil || result.TotalPages < 0 {
		return result, errors.New("invalid Beszel response")
	}
	return result, nil
}
func (b *beszelBackend) Read(ctx context.Context, now time.Time) (*History, error) {
	h := &History{Configured: true, Start: now.Add(-b.options.Window).Unix(), End: now.Unix(), Points: []Point{}}
	filter := fmt.Sprintf("%s && sample_time >= %d && sample_time <= %d", b.filter(), h.Start, h.End)
	// The collection's unique (server, sample_time) index bounds history size.
	limit := int(b.options.Window/time.Second) + 2
	for page := 1; ; page++ {
		data, e := b.list(ctx, filter, "sample_time", page, 500)
		if e != nil {
			return nil, e
		}
		for _, record := range data.Items {
			p := record.Point
			if record.Server != b.options.ServerID || record.SampleTime != p.Time || p.Time < h.Start || p.Time > h.End || !validPoint(p) {
				return nil, errors.New("invalid Beszel sample")
			}
			h.Points = append(h.Points, p)
		}
		if len(h.Points) > limit || page > (limit+499)/500 {
			return nil, errors.New("too many Beszel samples")
		}
		if page >= data.TotalPages {
			break
		}
	}
	sort.Slice(h.Points, func(i, j int) bool { return h.Points[i].Time < h.Points[j].Time })
	return h, nil
}
func validPoint(p Point) bool {
	switch p.State {
	case "active", "collecting", "paused", "disconnected", "unavailable":
	default:
		return false
	}
	for _, v := range []*float64{p.UPS, p.Target, p.Players} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0) {
			return false
		}
	}
	return p.State != "active" || p.UPS != nil
}
func (b *beszelBackend) Write(ctx context.Context, p Point) error {
	address, e := b.address()
	if e != nil {
		return e
	}
	data, e := json.Marshal(beszelRecord{Server: b.options.ServerID, SampleTime: p.Time, Point: p})
	if e != nil {
		return errors.New("cannot encode UPS sample")
	}
	if _, e = send(ctx, b.client, http.MethodPost, address, b.options.BeszelToken, data, map[string]string{"Content-Type": "application/json"}); e != nil {
		return e
	}
	if time.Now().Before(b.nextCleanup) {
		return nil
	}
	// A bounded batch prevents retention work from monopolizing the write worker.
	err := b.cleanup(ctx, address)
	if err != nil {
		return fmt.Errorf("sample stored; retention failed: %w", err)
	}
	return nil
}
func (b *beszelBackend) cleanup(ctx context.Context, address string) error {
	expired, e := b.list(ctx, fmt.Sprintf("%s && sample_time < %d", b.filter(), time.Now().Add(-b.options.Retention).Unix()), "sample_time", 1, 100)
	if e != nil {
		return e
	}
	for _, r := range expired.Items {
		if r.Server != b.options.ServerID || r.SampleTime >= time.Now().Add(-b.options.Retention).Unix() || !regexpID.MatchString(r.ID) {
			return errors.New("invalid Beszel retention record")
		}
		if _, e = send(ctx, b.client, http.MethodDelete, address+"/"+r.ID, b.options.BeszelToken, nil, nil); e != nil {
			return e
		}
	}
	if len(expired.Items) < 100 {
		b.nextCleanup = time.Now().Add(b.options.CleanupInterval)
	}
	return nil
}

var regexpID = regexp.MustCompile(`^[a-zA-Z0-9]{15}$`)
