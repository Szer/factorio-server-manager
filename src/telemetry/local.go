package telemetry

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

// Sample uses a monotonic observation time and a game-process generation.
type Sample struct {
	Tick     uint64
	Speed    float64
	Paused   bool
	Players  int
	Session  uint64
	Observed time.Time
}
type Probe func(context.Context) (Sample, error)
type Collector struct {
	Window, Freshness time.Duration
	mu                sync.RWMutex
	previous          *Sample
	points            []Point
}

func (c *Collector) Record(s Sample, err error, now time.Time) Point {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := Point{Time: now.Unix(), State: "disconnected"}
	if err == nil && (s.Observed.IsZero() || s.Speed < 0 || s.Players < 0 || math.IsNaN(s.Speed) || math.IsInf(s.Speed, 0)) {
		err = errors.New("invalid sample")
	}
	if err != nil {
		c.previous = nil
	} else {
		target, players := 60*s.Speed, float64(s.Players)
		p.Target, p.Players = &target, &players
		// A zero tick delta with no players distinguishes empty-server auto-pause.
		s.Paused = s.Paused || s.Speed == 0 || (c.previous != nil && s.Players == 0 && c.previous.Players == 0 && s.Tick == c.previous.Tick)
		p.State = "collecting"
		if s.Paused {
			p.State = "paused"
		} else if prev := c.previous; prev != nil && !prev.Paused && prev.Session == s.Session && prev.Speed == s.Speed && s.Tick >= prev.Tick {
			elapsed := s.Observed.Sub(prev.Observed).Seconds()
			if elapsed > 0 && elapsed <= c.freshness().Seconds() {
				ups := float64(s.Tick-prev.Tick) / elapsed
				p.UPS = &ups
				p.State = "active"
			}
		}
		c.previous = &s
	}
	c.append(p, now)
	return p
}
func (c *Collector) window() time.Duration {
	if c.Window > 0 {
		return c.Window
	}
	return 5 * time.Minute
}
func (c *Collector) freshness() time.Duration {
	if c.Freshness > 0 {
		return c.Freshness
	}
	return 25 * time.Second
}
func (c *Collector) append(p Point, now time.Time) {
	c.points = append(c.points, p)
	for len(c.points) > 0 && (c.points[0].Time < now.Add(-c.window()).Unix() || len(c.points) > int(c.window()/time.Second)+2) {
		c.points = c.points[1:]
	}
}
func (c *Collector) History(now time.Time) *History {
	c.mu.RLock()
	defer c.mu.RUnlock()
	h := &History{Configured: true, Start: now.Add(-c.window()).Unix(), End: now.Unix(), Points: []Point{}}
	for _, p := range c.points {
		if p.Time >= h.Start {
			h.Points = append(h.Points, p)
		}
	}
	if len(h.Points) == 0 || now.Unix()-h.Points[len(h.Points)-1].Time > int64(c.freshness()/time.Second) {
		h.Points = append(h.Points, Point{Time: now.Unix(), State: "unavailable"})
	}
	return h
}
