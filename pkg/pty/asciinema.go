package pty

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AsciinemaHeader defines the v2 metadata header record.
type AsciinemaHeader struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Title     string            `json:"title,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// ExportAsciinemaV2 converts a PTYSession into an asciinema v2 NDJSON byte stream.
func ExportAsciinemaV2(session *PTYSession) ([]byte, error) {
	var buf bytes.Buffer

	cols := session.Metadata.InitialSize.Cols
	rows := session.Metadata.InitialSize.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	header := AsciinemaHeader{
		Version:   2,
		Width:     cols,
		Height:    rows,
		Timestamp: session.Metadata.StartTime.Unix(),
		Title:     fmt.Sprintf("AgentVCR PTY: %s", session.Metadata.Command),
		Env:       session.Metadata.Env,
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("failed to encode asciinema header: %w", err)
	}

	buf.Write(headerBytes)
	buf.WriteByte('\n')

	for _, ev := range session.Events {
		eventTuple := []interface{}{
			ev.OffsetSec,
			string(ev.Type),
			ev.Data,
		}

		line, err := json.Marshal(eventTuple)
		if err != nil {
			continue
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}

	return buf.Bytes(), nil
}

// ImportAsciinemaV2 parses an asciinema v2 format byte stream into a PTYSession.
func ImportAsciinemaV2(data []byte) (*PTYSession, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty asciinema v2 file")
	}

	var header AsciinemaHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("invalid asciinema v2 header: %w", err)
	}

	startTime := time.Unix(header.Timestamp, 0).UTC()
	session := &PTYSession{
		Metadata: SessionMetadata{
			ID:          fmt.Sprintf("pty_%d", header.Timestamp),
			Command:     header.Title,
			StartTime:   startTime,
			InitialSize: WindowSize{Cols: header.Width, Rows: header.Height},
			Env:         header.Env,
		},
		Events: make([]PTYEvent, 0),
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw []interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		if len(raw) < 3 {
			continue
		}

		offset, ok1 := raw[0].(float64)
		evTypeStr, ok2 := raw[1].(string)
		evData, ok3 := raw[2].(string)

		if !ok1 || !ok2 || !ok3 {
			continue
		}

		ev := PTYEvent{
			Timestamp: startTime.Add(time.Duration(offset * float64(time.Second))),
			OffsetSec: offset,
			Type:      EventType(evTypeStr),
			Data:      evData,
		}
		session.Events = append(session.Events, ev)
	}

	if len(session.Events) > 0 {
		last := session.Events[len(session.Events)-1]
		session.Metadata.EndTime = last.Timestamp
	}

	return session, nil
}
