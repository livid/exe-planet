// Package planet holds Planet's own data shapes: the My/ library files a
// Planet app writes, the public planet.json and article.json a follower
// reads, and the encoding rules both share. A site built here must be
// byte for byte what Planet 0.22.4 builds, so nothing in this package is
// approximate.
package planet

import (
	"math"
	"strconv"
	"time"
)

// Date is Foundation's default Date encoding: seconds since
// 2001-01-01T00:00:00Z as a double. A Planet follower decodes nothing
// else, so it is kept as the number it was read as and only turned into
// a time.Time for display.
type Date float64

var reference = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// Time converts to a time.Time in UTC.
func (d Date) Time() time.Time {
	sec, frac := math.Modf(float64(d))
	return reference.Add(time.Duration(sec)*time.Second + time.Duration(math.Round(frac*1e9))*time.Nanosecond)
}

// DateOf converts a time.Time, keeping microseconds as Foundation does.
func DateOf(t time.Time) Date {
	d := t.Sub(reference)
	return Date(math.Round(d.Seconds()*1e6) / 1e6)
}

// String prints the number as Foundation's JSON does: no exponent, no
// trailing ".0" for a whole number, shortest digits otherwise.
func (d Date) String() string {
	return formatNumber(float64(d))
}

func formatNumber(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// IntervalSince1970 is Swift's Date.timeIntervalSince1970: the reference
// interval plus the 2001 offset, in double arithmetic.
func (d Date) IntervalSince1970() float64 { return float64(d) + 978307200.0 }

// IntervalSinceReferenceDate is the number itself.
func (d Date) IntervalSinceReferenceDate() float64 { return float64(d) }
