package telemetry

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

type Backend interface {
	Read(context.Context, time.Time) (*History, error)
	Write(context.Context, Point) error
}
type backendState struct {
	name     string
	backend  Backend
	queue    chan Point
	gate     chan struct{}
	failures int
	retryAt  time.Time
}
type Service struct {
	options  Options
	backends []*backendState
	now      func() time.Time
	mu       sync.Mutex
	source   string
	logMu    sync.Mutex
	logTimes map[string]time.Time
}

func NewService(o Options) *Service {
	implementations := map[string]Backend{}
	for _, name := range o.Backends {
		switch name {
		case "local":
			implementations[name] = &memoryBackend{collector: Collector{Window: o.Window, Freshness: o.Freshness}}
		case "prometheus":
			implementations[name] = newPrometheus(o)
		case "beszel":
			implementations[name] = newBeszel(o)
		}
	}
	return newService(o, implementations)
}
func newService(o Options, implementations map[string]Backend) *Service {
	s := &Service{options: o, now: time.Now, logTimes: map[string]time.Time{}}
	for _, name := range o.Backends {
		s.backends = append(s.backends, &backendState{name: name, backend: implementations[name], queue: make(chan Point, o.QueueSize), gate: make(chan struct{}, 1)})
	}
	return s
}
func (s *Service) Run(ctx context.Context, probe Probe) {
	var workers sync.WaitGroup
	for _, b := range s.backends {
		if b.name == "local" {
			continue
		}
		workers.Add(1)
		go func(b *backendState) { defer workers.Done(); s.writeLoop(ctx, b) }(b)
	}
	defer workers.Wait()
	calculator := Collector{Window: s.options.Window, Freshness: s.options.Freshness}
	ticker := time.NewTicker(s.options.Interval)
	defer ticker.Stop()
	previousState := ""
	for ctx.Err() == nil {
		sampleCtx, cancel := context.WithTimeout(ctx, s.options.ProbeTimeout)
		sample, err := probe(sampleCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		point := calculator.Record(sample, err, s.now())
		if point.State != previousState {
			s.event("sample:"+point.State, "sampler state=%s", point.State)
			previousState = point.State
		}
		s.Publish(ctx, point)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) Publish(ctx context.Context, p Point) {
	for _, b := range s.backends {
		if b.name == "local" {
			_ = b.backend.Write(ctx, p)
			continue
		}
		select {
		case b.queue <- p:
		default:
			s.event("drop:"+b.name, "backend=%s write queue full; dropping sample", b.name)
		}
	}
}
func (s *Service) writeLoop(ctx context.Context, b *backendState) {
	failed := false
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-b.queue:
			if ctx.Err() != nil {
				return
			}
			attempt, cancel := context.WithTimeout(ctx, s.options.WriteTimeout)
			err := b.backend.Write(attempt, p)
			cancel()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				s.event("write-fail:"+b.name, "backend=%s write failed: %v", b.name, err)
				failed = true
			} else if failed {
				s.event("write-recovery:"+b.name, "backend=%s writes recovered", b.name)
				failed = false
			}
		}
	}
}
func (s *Service) History(ctx context.Context, now time.Time) (*History, error) {
	ctx, cancel := context.WithTimeout(ctx, s.options.ReadBudget)
	defer cancel()
	for index, b := range s.backends {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Serialize reads per backend so recovery has only one in-flight probe.
		select {
		case b.gate <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		// Reserve part of the remaining budget for lower-priority backends.
		timeout := s.options.ReadTimeout
		if deadline, ok := ctx.Deadline(); ok {
			share := time.Until(deadline) / time.Duration(len(s.backends)-index)
			if share < timeout {
				timeout = share
			}
		}
		h, err := s.read(ctx, b, now, timeout)
		<-b.gate
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil {
			s.setSource(b.name)
			return s.withGaps(h), nil
		}
	}
	s.setSource("unavailable")
	return nil, errors.New("all UPS backends unavailable")
}
func (s *Service) read(ctx context.Context, b *backendState, now time.Time, timeout time.Duration) (*History, error) {
	if !b.retryAt.IsZero() && s.now().Before(b.retryAt) {
		return nil, errors.New("read circuit open")
	}
	attempt, cancel := context.WithTimeout(ctx, timeout)
	h, err := b.backend.Read(attempt, now)
	cancel()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err == nil && !fresh(h, now, s.options.Freshness) {
		err = errors.New("no fresh samples")
	}
	if err == nil {
		if b.failures > 0 {
			s.event("read-recovery:"+b.name, "backend=%s reads recovered", b.name)
		}
		b.failures = 0
		b.retryAt = time.Time{}
		return h, nil
	}
	b.failures++
	s.event("read-fail:"+b.name, "backend=%s read failed: %v", b.name, err)
	if b.failures >= s.options.Failures {
		if b.retryAt.IsZero() {
			s.event("circuit:"+b.name, "backend=%s read circuit opened; cooldown=%s", b.name, s.options.Cooldown)
		}
		b.retryAt = s.now().Add(s.options.Cooldown)
	}
	return nil, err
}
func fresh(h *History, now time.Time, age time.Duration) bool {
	if h == nil {
		return false
	}
	for _, p := range h.Points {
		if p.State != "unavailable" && p.Time <= now.Unix() && p.Time >= now.Add(-age).Unix() {
			return true
		}
	}
	return false
}
func (s *Service) setSource(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.source != name {
		s.event("source:"+name, "read source=%s", name)
		s.source = name
	}
}

type memoryBackend struct{ collector Collector }

func (b *memoryBackend) Read(_ context.Context, now time.Time) (*History, error) {
	return b.collector.History(now), nil
}
func (b *memoryBackend) Write(_ context.Context, p Point) error {
	b.collector.mu.Lock()
	defer b.collector.mu.Unlock()
	b.collector.append(p, time.Unix(p.Time, 0))
	return nil
}

// Repeated transitions are rate-limited by event and backend, including flapping.
func (s *Service) event(key, format string, args ...interface{}) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	now := s.now()
	if last, ok := s.logTimes[key]; ok && now.Sub(last) < s.options.LogInterval {
		return
	}
	log.Printf("UPS: "+format, args...)
	s.logTimes[key] = now
}

// Missing writes remain gaps rather than implying continuous performance.
func (s *Service) withGaps(h *History) *History {
	copy := *h
	copy.Points = make([]Point, 0, len(h.Points))
	var previous *Point
	for _, point := range h.Points {
		if previous != nil && point.Time-previous.Time >= int64(2*s.options.Interval/time.Second) {
			copy.Points = append(copy.Points, Point{Time: previous.Time + int64(s.options.Interval/time.Second), State: "unavailable"})
		}
		copy.Points = append(copy.Points, point)
		previous = &copy.Points[len(copy.Points)-1]
	}
	return &copy
}

type ClientSettings struct {
	PollIntervalMS   int64 `json:"poll_interval_ms"`
	RequestTimeoutMS int64 `json:"request_timeout_ms"`
}

func (s *Service) ClientSettings() ClientSettings {
	return ClientSettings{PollIntervalMS: s.options.Interval.Milliseconds(), RequestTimeoutMS: (s.options.ReadBudget + s.options.ReadTimeout).Milliseconds()}
}
