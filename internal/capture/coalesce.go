package capture

// Coalescer collapses a stream of identical samples into durable sessions.
//
// Polling frontmost-app every 5s produces ~17k rows per day per source, almost
// all repeats. Coalescing happens on write, not on read: storing the raw stream
// and aggregating at query time costs disk and query latency forever in
// exchange for nothing.
type Coalescer struct {
	open *Event
	// maxGapS: if a sample arrives after a gap much longer than the poll
	// interval, the machine was probably asleep. Closing the session at its
	// last known sample avoids inventing an 8-hour "session" over a lunch break.
	maxGapS int64
}

func NewCoalescer(maxGapS int64) *Coalescer {
	return &Coalescer{maxGapS: maxGapS}
}

// Push feeds one sample, returning a completed session if this sample ended one.
func (c *Coalescer) Push(sample Event) *Event {
	if c.open == nil {
		s := sample
		c.open = &s
		return nil
	}

	gap := sample.TS - (c.open.TS + c.open.DurS)
	same := c.open.Identity() == sample.Identity()

	if same && gap <= c.maxGapS {
		c.open.DurS = sample.TS - c.open.TS
		return nil
	}

	done := *c.open
	if !same && gap <= c.maxGapS {
		// State changed cleanly, so the old session ran right up to this sample
		// rather than stopping at the last poll.
		done.DurS = sample.TS - done.TS
	}

	s := sample
	c.open = &s
	return &done
}

// Flush closes the session in flight, e.g. on shutdown.
func (c *Coalescer) Flush() *Event {
	if c.open == nil {
		return nil
	}
	done := *c.open
	c.open = nil
	if done.DurS == 0 {
		if d := Now() - done.TS; d > 0 {
			done.DurS = d
		}
	}
	return &done
}

func (c *Coalescer) Open() *Event { return c.open }
