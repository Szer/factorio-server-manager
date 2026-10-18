package telemetry

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeBackend struct {
	read  func(context.Context, time.Time) (*History, error)
	write func(context.Context, Point) error
}

func (b *fakeBackend) Read(c context.Context, n time.Time) (*History, error) { return b.read(c, n) }
func (b *fakeBackend) Write(c context.Context, p Point) error {
	if b.write != nil {
		return b.write(c, p)
	}
	return nil
}
func testOptions(t *testing.T) Options {
	t.Helper()
	t.Setenv("UPS_BACKENDS", "local")
	return LoadOptions("", nil)
}
func healthy(_ context.Context, n time.Time) (*History, error) {
	return &History{Configured: true, Points: []Point{{Time: n.Unix(), State: "paused"}}}, nil
}
func TestOrderedReadCircuitAndRecovery(t *testing.T) {
	o := testOptions(t)
	o.Backends = []string{"first", "second"}
	o.Failures = 2
	now := time.Now()
	calls := 0
	broken := true
	first := &fakeBackend{read: func(c context.Context, n time.Time) (*History, error) {
		calls++
		if broken {
			return nil, errors.New("offline")
		}
		return healthy(c, n)
	}}
	s := newService(o, map[string]Backend{"first": first, "second": &fakeBackend{read: healthy}})
	s.now = func() time.Time { return now }
	for i := 0; i < 4; i++ {
		if _, e := s.History(context.Background(), now); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 2 || s.source != "second" {
		t.Fatalf("calls=%d source=%s", calls, s.source)
	}
	now = now.Add(o.Cooldown)
	broken = false
	if _, e := s.History(context.Background(), now); e != nil {
		t.Fatal(e)
	}
	if calls != 3 || s.source != "first" || s.backends[0].failures != 0 {
		t.Fatal("did not recover priority")
	}
}
func TestTimeoutFallbackAndCancellation(t *testing.T) {
	o := testOptions(t)
	o.Backends = []string{"slow", "local"}
	o.ReadTimeout = 10 * time.Millisecond
	slow := &fakeBackend{read: func(c context.Context, _ time.Time) (*History, error) { <-c.Done(); return nil, c.Err() }}
	s := newService(o, map[string]Backend{"slow": slow, "local": &fakeBackend{read: healthy}})
	if _, e := s.History(context.Background(), time.Now()); e != nil {
		t.Fatal(e)
	}
	if s.backends[0].failures != 1 || s.source != "local" {
		t.Fatal("timeout did not fall back")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.History(ctx, time.Now()); e == nil {
		t.Fatal("canceled caller accepted")
	}
	if s.backends[0].failures != 1 {
		t.Fatal("caller cancellation tripped circuit")
	}
}
func TestConcurrentHealthyReadsWait(t *testing.T) {
	o := testOptions(t)
	o.Backends = []string{"one"}
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	b := &fakeBackend{read: func(c context.Context, n time.Time) (*History, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return healthy(c, n)
	}}
	s := newService(o, map[string]Backend{"one": b})
	results := make(chan error, 2)
	go func() { _, e := s.History(context.Background(), time.Now()); results <- e }()
	<-entered
	go func() { _, e := s.History(context.Background(), time.Now()); results <- e }()
	close(release)
	for i := 0; i < 2; i++ {
		if e := <-results; e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("healthy backend skipped")
	}
}
func TestStaleFallbackButDisconnectedIsHealthy(t *testing.T) {
	o := testOptions(t)
	o.Backends = []string{"stale", "fresh"}
	stale := &fakeBackend{read: func(c context.Context, n time.Time) (*History, error) { return healthy(c, n.Add(-time.Hour)) }}
	disconnected := &fakeBackend{read: func(_ context.Context, n time.Time) (*History, error) {
		return &History{Points: []Point{{Time: n.Unix(), State: "disconnected"}}}, nil
	}}
	s := newService(o, map[string]Backend{"stale": stale, "fresh": disconnected})
	if _, e := s.History(context.Background(), time.Now()); e != nil || s.source != "fresh" {
		t.Fatal("fresh stopped state should be healthy", e)
	}
}
func TestIndependentBoundedWriters(t *testing.T) {
	o := testOptions(t)
	o.Backends = []string{"slow", "fast", "local"}
	o.QueueSize = 1
	entered := make(chan struct{})
	fast := make(chan struct{}, 100)
	var once sync.Once
	slow := &fakeBackend{write: func(c context.Context, _ Point) error { once.Do(func() { close(entered) }); <-c.Done(); return c.Err() }}
	quick := &fakeBackend{write: func(context.Context, Point) error { fast <- struct{}{}; return nil }}
	local := &memoryBackend{collector: Collector{Window: o.Window, Freshness: o.Freshness}}
	s := newService(o, map[string]Backend{"slow": slow, "fast": quick, "local": local})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for _, b := range s.backends[:2] {
		wg.Add(1)
		go func(b *backendState) { defer wg.Done(); s.writeLoop(ctx, b) }(b)
	}
	p := Point{Time: time.Now().Unix(), State: "paused"}
	s.Publish(ctx, p)
	<-entered
	select {
	case <-fast:
	case <-time.After(time.Second):
		t.Fatal("slow writer blocked fast writer")
	}
	for i := 0; i < 10; i++ {
		s.Publish(ctx, p)
	}
	if len(s.backends[0].queue) > 1 {
		t.Fatal("unbounded queue")
	}
	if !fresh(local.collector.History(time.Now()), time.Now(), o.Freshness) {
		t.Fatal("local write blocked")
	}
	cancel()
	wg.Wait()
}
func TestEnvValidationAndLegacy(t *testing.T) {
	t.Setenv("UPS_BACKENDS", " beszel, local,beszel,wrong,prometheus ")
	t.Setenv("UPS_READ_TIMEOUT", "-1s")
	t.Setenv("UPS_SAMPLE_INTERVAL", "1ms")
	t.Setenv("UPS_WRITE_QUEUE_SIZE", "0")
	o := LoadOptions("http://prometheus", nil)
	if len(o.Backends) != 3 || o.Backends[0] != "beszel" || o.Backends[1] != "local" || o.ReadTimeout != 3*time.Second || o.Interval != 10*time.Second || o.QueueSize != 6 || o.LegacyPrometheus {
		t.Fatal(o)
	}
}

func TestCancellationDuringReadDoesNotTripBreaker(t *testing.T) {
	o := testOptions(t)
	o.Backends = []string{"one"}
	entered := make(chan struct{})
	b := &fakeBackend{read: func(c context.Context, _ time.Time) (*History, error) {
		close(entered)
		<-c.Done()
		return nil, c.Err()
	}}
	s := newService(o, map[string]Backend{"one": b})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := s.History(ctx, time.Now()); done <- e }()
	<-entered
	cancel()
	<-done
	if s.backends[0].failures != 0 {
		t.Fatal("caller cancellation poisoned health")
	}
}
func TestHalfOpenHasSingleFailingProbe(t *testing.T) {
	o := testOptions(t)
	o.Backends = []string{"one", "local"}
	o.Failures = 1
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	b := &fakeBackend{read: func(context.Context, time.Time) (*History, error) {
		calls.Add(1)
		close(entered)
		<-release
		return nil, errors.New("still down")
	}}
	s := newService(o, map[string]Backend{"one": b, "local": &fakeBackend{read: healthy}})
	s.backends[0].failures = 1
	s.backends[0].retryAt = time.Now().Add(-time.Second)
	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { _, e := s.History(context.Background(), time.Now()); done <- e }()
	}
	<-entered
	close(release)
	for i := 0; i < 4; i++ {
		if e := <-done; e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("multiple recovery probes", calls.Load())
	}
}

func TestSmallTotalBudgetStillAllowsFallback(t *testing.T) {
	o := testOptions(t)
	o.Backends = []string{"slow", "local"}
	o.ReadTimeout = time.Second
	o.ReadBudget = 40 * time.Millisecond
	slow := &fakeBackend{read: func(c context.Context, _ time.Time) (*History, error) { <-c.Done(); return nil, c.Err() }}
	s := newService(o, map[string]Backend{"slow": slow, "local": &fakeBackend{read: healthy}})
	if _, e := s.History(context.Background(), time.Now()); e != nil || s.source != "local" {
		t.Fatal("primary consumed entire budget", e)
	}
}
func TestMissingSampleCreatesChartGap(t *testing.T) {
	o := testOptions(t)
	s := newService(o, map[string]Backend{"local": &fakeBackend{read: healthy}})
	h := s.withGaps(&History{Points: []Point{{Time: 100, State: "active"}, {Time: 120, State: "active"}}})
	if len(h.Points) != 3 || h.Points[1].Time != 110 || h.Points[1].State != "unavailable" {
		t.Fatal(h)
	}
}
