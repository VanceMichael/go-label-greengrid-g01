package clock

import "time"

type Clock interface{ Now() time.Time }
type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

type Fixed struct{ Value time.Time }

func (f Fixed) Now() time.Time             { return f.Value.UTC() }
func Window(start, end, at time.Time) bool { return !at.Before(start) && at.Before(end) }
func Expired(deadline, at time.Time) bool  { return !at.Before(deadline) }
