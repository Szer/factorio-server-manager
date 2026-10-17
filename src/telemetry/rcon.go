package telemetry

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"time"
)

const sampleCommand = `/sc rcon.print(game.tick .. " " .. game.speed .. " " .. tostring(game.tick_paused) .. " " .. #game.connected_players)`

// ReadRCON uses its own bounded connection, separate from interactive commands.
func ReadRCON(ctx context.Context, address, password string) (Sample, error) {
	var result Sample
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return result, errors.New("RCON unavailable")
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	if err = sendPacket(conn, 1, 3, password); err != nil {
		return result, err
	}
	authenticated := false
	for i := 0; i < 4; i++ {
		id, kind, _, e := readPacket(conn)
		if e != nil {
			return result, e
		}
		if id == -1 {
			return result, errors.New("RCON authentication failed")
		}
		if id == 1 && kind == 2 {
			authenticated = true
			break
		}
	}
	if !authenticated {
		return result, errors.New("invalid RCON authentication response")
	}
	started := time.Now()
	if err = sendPacket(conn, 2, 2, sampleCommand); err != nil {
		return result, err
	}
	for i := 0; i < 4; i++ {
		id, kind, body, e := readPacket(conn)
		if e != nil {
			return result, e
		}
		if id == 2 && kind == 0 {
			return parseSample(body, started.Add(time.Since(started)/2))
		}
	}
	return result, errors.New("missing RCON sample")
}
func sendPacket(w io.Writer, id, kind int32, body string) error {
	packet := make([]byte, 14+len(body))
	binary.LittleEndian.PutUint32(packet, uint32(len(packet)-4))
	binary.LittleEndian.PutUint32(packet[4:], uint32(id))
	binary.LittleEndian.PutUint32(packet[8:], uint32(kind))
	copy(packet[12:], body)
	_, err := io.Copy(w, strings.NewReader(string(packet)))
	return err
}
func readPacket(r io.Reader) (int32, int32, string, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return 0, 0, "", err
	}
	if length < 10 || length > 4096 {
		return 0, 0, "", errors.New("invalid RCON packet size")
	}
	body := make([]byte, int(length))
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, 0, "", err
	}
	if body[len(body)-1] != 0 || body[len(body)-2] != 0 {
		return 0, 0, "", errors.New("invalid RCON packet terminator")
	}
	return int32(binary.LittleEndian.Uint32(body)), int32(binary.LittleEndian.Uint32(body[4:])), string(body[8 : len(body)-2]), nil
}
func parseSample(body string, observed time.Time) (Sample, error) {
	s := Sample{Observed: observed}
	fields := strings.Fields(body)
	if len(fields) != 4 {
		return s, errors.New("invalid RCON sample")
	}
	var err error
	if s.Tick, err = strconv.ParseUint(fields[0], 10, 64); err != nil {
		return s, errors.New("invalid tick")
	}
	if s.Speed, err = strconv.ParseFloat(fields[1], 64); err != nil || math.IsNaN(s.Speed) || math.IsInf(s.Speed, 0) || s.Speed < 0 {
		return s, errors.New("invalid speed")
	}
	if fields[2] != "true" && fields[2] != "false" {
		return s, errors.New("invalid pause state")
	}
	s.Paused = fields[2] == "true"
	if s.Players, err = strconv.Atoi(fields[3]); err != nil || s.Players < 0 {
		return s, fmt.Errorf("invalid player count")
	}
	return s, nil
}
