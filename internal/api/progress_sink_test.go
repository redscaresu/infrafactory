package api

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedEvent is what would go over the wire.
type recordedEvent struct {
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}

// sinkFor wires a sink to a hub with one subscriber, and returns a
// reader for whatever has been broadcast so far.
//
// The reader DRAINS: call it once per assertion block and keep the
// result.
//
// The subscriber is a Client with only its send channel: Broadcast never
// touches the connection, so a test does not need a real websocket to
// observe what a client would receive.
func sinkFor(t *testing.T, subject string) (*ProgressSink, func() []recordedEvent) {
	t.Helper()

	hub := NewHub()
	client := &Client{send: make(chan []byte, 256)}
	hub.Register(client)

	sink := NewProgressSink(hub, "deploy_progress", subject)

	return sink, func() []recordedEvent {
		var out []recordedEvent
		for {
			select {
			case raw := <-client.send:
				var e recordedEvent
				require.NoError(t, json.Unmarshal(raw, &e))
				out = append(out, e)
			default:
				return out
			}
		}
	}
}

// A command writing with several Fprintf calls produces fragments, and a
// page appending fragments shows half a word then the rest of it on the
// next row.
func TestProgressSinkEmitsWholeLinesNotWrites(t *testing.T) {
	sink, events := sinkFor(t, "lb-serving-paris")

	_, _ = sink.Write([]byte("Deploying lb-"))
	_, _ = sink.Write([]byte("serving-paris\nworkdir: /tmp/x\n"))

	got := events()
	require.Len(t, got, 2)
	assert.Equal(t, "Deploying lb-serving-paris", got[0].Data["line"])
	assert.Equal(t, "workdir: /tmp/x", got[1].Data["line"])
}

// A deploy takes minutes and the reader can navigate during it, so these
// events arrive on whatever page is open. An unattributed line is a
// statement about something they may not be looking at.
func TestProgressSinkNamesItsSubjectOnEveryLine(t *testing.T) {
	sink, events := sinkFor(t, "lb-serving-paris")

	_, _ = sink.Write([]byte("one\ntwo\n"))

	for _, e := range events() {
		assert.Equal(t, "lb-serving-paris", e.Data["subject"])
		assert.Equal(t, "deploy_progress", e.Type)
	}
}

// A command's last line often has no trailing newline, and it is
// frequently the one that matters.
func TestProgressSinkFlushesTheLastLineOnClose(t *testing.T) {
	sink, events := sinkFor(t, "lb-serving-paris")

	_, _ = sink.Write([]byte("Deployed as dep-20260902"))
	require.NoError(t, sink.Close())

	got := events()
	require.Len(t, got, 1)
	assert.Equal(t, "Deployed as dep-20260902", got[0].Data["line"])
}

func TestProgressSinkCloseIsQuietWhenNothingIsBuffered(t *testing.T) {
	sink, events := sinkFor(t, "x")

	_, _ = sink.Write([]byte("done\n"))
	require.NoError(t, sink.Close())

	assert.Len(t, events(), 1, "close must not repeat the last line")
}

// Blank lines carry no information and would pad the stream.
func TestProgressSinkDropsEmptyLines(t *testing.T) {
	sink, events := sinkFor(t, "x")

	_, _ = sink.Write([]byte("\n\nreal\n\n"))

	got := events()
	require.Len(t, got, 1)
	assert.Equal(t, "real", got[0].Data["line"])
}

// Windows-style line endings must not leave a stray carriage return that
// a browser renders as a box.
func TestProgressSinkTrimsCarriageReturns(t *testing.T) {
	sink, events := sinkFor(t, "x")

	_, _ = sink.Write([]byte("line\r\n"))

	// Captured once: the reader drains the channel, so calling it twice
	// returns nothing the second time.
	got := events()
	require.Len(t, got, 1)
	assert.Equal(t, "line", got[0].Data["line"])
}

// A nil sink is a no-op rather than a panic: the caller passes one when
// there is no hub, and a deploy must not die because nobody is watching.
func TestNilProgressSinkIsHarmless(t *testing.T) {
	var sink *ProgressSink

	n, err := sink.Write([]byte("anything"))

	require.NoError(t, err)
	assert.Equal(t, len("anything"), n)
	require.NoError(t, sink.Close())
}

// Concurrent writers must not interleave inside a line.
func TestProgressSinkIsSafeUnderConcurrentWrites(t *testing.T) {
	sink, events := sinkFor(t, "x")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sink.Write([]byte("aaaa\n"))
		}()
	}
	wg.Wait()

	got := events()
	assert.Len(t, got, 20)
	for _, e := range got {
		assert.Equal(t, "aaaa", e.Data["line"], "a line must not be interleaved with another")
	}
}
