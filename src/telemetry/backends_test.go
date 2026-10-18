package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func TestPrometheusRemoteWrite(t *testing.T) {
	for _, state := range []string{"active", "paused", "disconnected", "collecting"} {
		t.Run(state, func(t *testing.T) {
			o := testOptions(t)
			o.PrometheusToken = "Bearer test"
			o.Labels = map[string]string{"server": "test"}
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.URL.Path != "/api/v1/write" || r.Header.Get("Authorization") != "Bearer test" || r.Header.Get("Content-Encoding") != "snappy" || r.Header.Get("X-Prometheus-Remote-Write-Version") != "0.1.0" {
					t.Error("bad remote-write request")
				}
				raw, _ := io.ReadAll(r.Body)
				decoded, e := snappy.Decode(nil, raw)
				if e != nil {
					t.Error(e)
					return
				}
				var request prompb.WriteRequest
				if e = request.Unmarshal(decoded); e != nil {
					t.Error(e)
					return
				}
				values := map[string]float64{}
				for _, series := range request.Timeseries {
					if len(series.Labels) != 2 || series.Labels[0].Name != "__name__" || series.Samples[0].Timestamp != 1000000 {
						t.Error("bad sample")
					}
					values[series.Labels[0].Value] = series.Samples[0].Value
				}
				if len(values) != 6 || values["factorio_sample_timestamp_seconds"] != 1000 {
					t.Error(values)
				}
				if state == "active" {
					if values["factorio_ups"] != 60 {
						t.Error(values)
					}
				} else if !math.IsNaN(values["factorio_ups"]) {
					t.Error("gap carried old UPS")
				}
				if state == "disconnected" && values["factorio_rcon_up"] != 0 {
					t.Error("not disconnected")
				}
				if state == "paused" && values["factorio_paused"] != 1 {
					t.Error("not paused")
				}
				w.WriteHeader(204)
			}))
			defer server.Close()
			o.PrometheusWriteURL = server.URL + "/api/v1/write"
			b := newPrometheus(o)
			p := Point{Time: 1000, State: state}
			v := 60.0
			if state == "active" {
				p.UPS = &v
			}
			if e := b.Write(context.Background(), p); e != nil || !called {
				t.Fatal(e)
			}
		})
	}
}
func TestBeszelRoundTripAndRetention(t *testing.T) {
	o := testOptions(t)
	o.BeszelToken = "test-token"
	o.ServerID = "test"
	var stored beszelRecord
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-token" {
			t.Error("missing token")
		}
		switch r.Method {
		case "POST":
			if json.NewDecoder(r.Body).Decode(&stored) != nil {
				t.Error("invalid JSON")
			}
			w.Write([]byte(`{}`))
		case "GET":
			filter := r.URL.Query().Get("filter")
			if !strings.Contains(filter, `server = "test"`) {
				t.Error("unscoped request")
			}
			records := []beszelRecord{stored}
			if strings.Contains(filter, "sample_time < ") {
				records = []beszelRecord{{ID: "aaaaaaaaaaaaaaa", Server: "test", SampleTime: time.Now().Add(-2 * o.Retention).Unix()}}
			}
			json.NewEncoder(w).Encode(beszelPage{Items: records, TotalPages: 1})
		case "DELETE":
			deleted = true
			if !strings.HasSuffix(r.URL.Path, "/aaaaaaaaaaaaaaa") {
				t.Error("wrong deletion")
			}
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	o.BeszelURL = server.URL
	b := newBeszel(o)
	p := Point{Time: time.Now().Unix(), State: "paused"}
	if e := b.Write(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	h, e := b.Read(context.Background(), time.Now())
	if e != nil || len(h.Points) != 1 || h.Points[0].State != "paused" || !deleted {
		t.Fatal(h, e, deleted)
	}
}
func TestBeszelFailuresAndValidation(t *testing.T) {
	for _, response := range []struct {
		code int
		body string
	}{{401, `secret-detail`}, {200, `{}`}, {200, `{"items":[{"server":"other","sample_time":1000,"point":{"time":1000,"state":"active","ups":60}}],"totalPages":1}`}} {
		o := testOptions(t)
		o.BeszelToken = "token"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(response.code)
			w.Write([]byte(response.body))
		}))
		o.BeszelURL = server.URL
		_, e := newBeszel(o).Read(context.Background(), time.Unix(1000, 0))
		server.Close()
		if e == nil || strings.Contains(e.Error(), "secret-detail") {
			t.Fatal("failure was accepted or leaked", e)
		}
	}
}

func TestPrometheusStaleDisconnectedFallsBack(t *testing.T) {
	o := testOptions(t)
	now := time.Unix(1000, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "data": map[string]interface{}{"resultType": "matrix", "result": []Series{metric("factorio_rcon_up", 990, "0", 1000, "0"), metric("factorio_sample_timestamp_seconds", 990, "960", 1000, "960")}}})
	}))
	defer server.Close()
	o.PrometheusURL = server.URL
	if _, e := newPrometheus(o).Read(context.Background(), now); e == nil {
		t.Fatal("accepted old disconnected sample re-timestamped by query_range")
	}
}
func TestConfiguredPrometheusTiming(t *testing.T) {
	o := testOptions(t)
	o.Interval = 15 * time.Second
	o.Window = 10 * time.Minute
	o.Freshness = 45 * time.Second
	o.ReadTimeout = 2 * time.Second
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("step") != "15" || q.Get("lookback_delta") != "45" || q.Get("timeout") != "2" || q.Get("start") != "390" {
			t.Error(q)
		}
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer server.Close()
	if _, e := fetch(context.Background(), server.Client(), server.URL, nil, time.Unix(1000, 0), o); e != nil {
		t.Fatal(e)
	}
}
func TestRetentionFullBatchRetriesWithoutCooldown(t *testing.T) {
	o := testOptions(t)
	o.BeszelToken = "test"
	deletes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			items := make([]beszelRecord, 100)
			for i := range items {
				items[i] = beszelRecord{ID: "aaaaaaaaaaaaaaa", Server: o.ServerID, SampleTime: 1}
			}
			json.NewEncoder(w).Encode(beszelPage{Items: items, TotalPages: 2})
			return
		}
		if r.Method == "DELETE" {
			deletes++
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	o.BeszelURL = server.URL
	b := newBeszel(o)
	if e := b.Write(context.Background(), Point{Time: time.Now().Unix(), State: "paused"}); e != nil {
		t.Fatal(e)
	}
	if deletes != 100 || !b.nextCleanup.IsZero() {
		t.Fatal("backlog should retry on next sample", deletes, b.nextCleanup)
	}
}

// The opt-in test uses a disposable, preprovisioned restricted PocketBase account.
func TestBeszelLive(t *testing.T) {
	address := os.Getenv("UPS_TEST_BESZEL_URL")
	if address == "" {
		t.Skip("no disposable Beszel instance")
	}
	o := testOptions(t)
	o.BeszelURL = address
	o.BeszelToken = os.Getenv("UPS_TEST_BESZEL_TOKEN")
	o.ServerID = "integration"
	b := newBeszel(o)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := Point{Time: time.Now().Unix(), State: "paused"}
	if e := b.Write(ctx, p); e != nil {
		t.Fatal(e)
	}
	h, e := b.Read(ctx, time.Now())
	if e != nil || !fresh(h, time.Now(), o.Freshness) {
		t.Fatal("live round trip failed", e)
	}
	// The token must not write records for another server.
	o.ServerID = "unauthorized"
	other := newBeszel(o)
	if e := other.Write(ctx, p); e == nil {
		t.Fatal("cross-server write accepted")
	}
}
