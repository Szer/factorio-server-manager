package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func metric(name string, values ...interface{}) Series {
	s := Series{Metric: map[string]string{"__name__": name}}
	for i := 0; i < len(values); i += 2 {
		a, _ := json.Marshal(values[i])
		b, _ := json.Marshal(values[i+1])
		s.Values = append(s.Values, [2]json.RawMessage{a, b})
	}
	return s
}
func TestStatesAndGaps(t *testing.T) {
	series := []Series{
		metric("factorio_rcon_up", 0, "1", 10, "1", 20, "1", 30, "1", 40, "0", 50, "1"),
		metric("factorio_sample_timestamp_seconds", 0, "0", 10, "10", 20, "20", 30, "-10", 50, "50"),
		metric("factorio_paused", 0, "0", 10, "1", 20, "0", 30, "0", 50, "0"),
		metric("factorio_ups", 0, "38.5", 10, "0", 30, "39", 50, "NaN"),
		metric("factorio_target_ups", 0, "39", 10, "39", 20, "60", 30, "39", 50, "60"),
	}
	points, e := normalize(series, 0, 60)
	if e != nil {
		t.Fatal(e)
	}
	states := []string{"active", "paused", "collecting", "unavailable", "disconnected", "collecting", "unavailable"}
	for i, state := range states {
		if points[i].State != state {
			t.Fatalf("%d: got %s expected %s", i, points[i].State, state)
		}
		if i > 0 && points[i].UPS != nil {
			t.Fatalf("invalid UPS at %d", i)
		}
	}
	if *points[0].UPS != 38.5 || *points[0].Target != 39 || points[3].Target != nil {
		t.Fatal("values or freshness incorrect")
	}
}
func TestDuplicateExportersRejected(t *testing.T) {
	_, e := normalize([]Series{metric("factorio_ups", 0, "30"), metric("factorio_ups", 0, "40")}, 0, 0)
	if e == nil {
		t.Fatal("ambiguous exporters accepted")
	}
}
func TestFetchBoundsAndLabels(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		q := r.URL.Query()
		if r.URL.Path != "/api/v1/query_range" || q.Get("start") != "700" || q.Get("end") != "1000" || q.Get("step") != "10s" || q.Get("lookback_delta") != "20s" || !strings.Contains(q.Get("query"), `service="test-exporter"`) {
			t.Errorf("unexpected query %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer server.Close()
	h, e := Fetch(context.Background(), server.Client(), server.URL, map[string]string{"service": "test-exporter"}, time.Unix(1007, 0))
	if e != nil || !called || len(h.Points) != 31 || h.Points[30].State != "unavailable" {
		t.Fatal("invalid history", h, e)
	}
}
func TestFailureAndDisabled(t *testing.T) {
	h, e := Fetch(context.Background(), http.DefaultClient, "", nil, time.Now())
	if e != nil || h.Configured {
		t.Fatal("disabled telemetry failed")
	}
	for _, body := range []string{`{"status":"error"}`, `not-json`, `{"status":"success","data":{"resultType":"vector"}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		_, e = Fetch(context.Background(), server.Client(), server.URL, nil, time.Now())
		server.Close()
		if e == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = Fetch(ctx, http.DefaultClient, "http://localhost:1", nil, time.Now()); e == nil {
		t.Fatal("ignored cancellation")
	}
}
