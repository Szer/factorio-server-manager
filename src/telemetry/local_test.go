package telemetry

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestLocalHistory(t *testing.T) {
	c := &Collector{}
	start := time.Now()
	record := func(seconds int, tick uint64, players int, paused bool, session uint64, err error) Point {
		now := start.Add(time.Duration(seconds) * time.Second)
		c.Record(Sample{Tick: tick, Speed: 1, Players: players, Paused: paused, Session: session, Observed: now}, err, now)
		h := c.History(now)
		return h.Points[len(h.Points)-1]
	}
	if p := record(0, 0, 1, false, 1, nil); p.UPS != nil {
		t.Fatal("first sample has rate")
	}
	if p := record(10, 600, 1, false, 1, nil); p.UPS == nil || *p.UPS != 60 {
		t.Fatal(p)
	}
	if p := record(20, 600, 0, true, 1, nil); p.UPS != nil || p.State != "paused" {
		t.Fatal(p)
	}
	if p := record(30, 1200, 1, false, 1, nil); p.UPS != nil {
		t.Fatal("rate spans pause")
	}
	record(40, 1800, 1, false, 1, errors.New("offline"))
	if p := record(50, 2400, 1, false, 1, nil); p.UPS != nil {
		t.Fatal("rate spans disconnect")
	}
	if p := record(60, 3000, 1, false, 2, nil); p.UPS != nil {
		t.Fatal("rate spans restart")
	}
	if p := record(70, 100, 1, false, 2, nil); p.UPS != nil {
		t.Fatal("rate spans save reload")
	}
	for i := 80; i <= 600; i += 10 {
		record(i, uint64(i*60), 1, false, 2, nil)
	}
	h := c.History(start.Add(600 * time.Second))
	if len(h.Points) > 31 || h.Points[0].Time < h.Start {
		t.Fatal("unbounded history")
	}
	h = c.History(start.Add(640 * time.Second))
	if h.Points[len(h.Points)-1].State != "unavailable" {
		t.Fatal("stale value")
	}
}
func TestEmptyServerPause(t *testing.T) {
	c := &Collector{}
	now := time.Now()
	for i := 0; i < 3; i++ {
		s := Sample{Tick: 42, Speed: 1, Players: 0, Observed: now.Add(time.Duration(i) * 10 * time.Second)}
		c.Record(s, nil, s.Observed)
	}
	h := c.History(now.Add(20 * time.Second))
	if p := h.Points[2]; p.State != "paused" || p.UPS != nil {
		t.Fatal(p)
	}
}
func TestRCONProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			done <- e
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		id, kind, body, e := readPacket(conn)
		if e != nil || id != 1 || kind != 3 || body != "test-only" {
			done <- errors.New("bad auth request")
			return
		}
		sendPacket(conn, 1, 0, "")
		sendPacket(conn, 1, 2, "")
		id, kind, body, e = readPacket(conn)
		if e != nil || id != 2 || kind != 2 || body != sampleCommand {
			done <- errors.New("bad sample request")
			return
		}
		done <- sendPacket(conn, 2, 0, "1200 0.65 false 2\n")
	}()
	s, err := ReadRCON(context.Background(), listener.Addr().String(), "test-only")
	if err != nil || s.Tick != 1200 || s.Speed != 0.65 || s.Players != 2 {
		t.Fatalf("unexpected sample %+v: %v", s, err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
func TestMalformedSamples(t *testing.T) {
	for _, s := range []string{"", "1 NaN false 0", "1 1 maybe 0", "1 1 false -1", "-1 1 false 0", "1 +Inf false 0"} {
		if _, err := parseSample(s, time.Now()); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}
func TestProbeTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		conn, e := listener.Accept()
		if e == nil {
			defer conn.Close()
			<-done
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := ReadRCON(ctx, listener.Addr().String(), "test-only"); err == nil {
		t.Fatal("missing timeout")
	}
}
