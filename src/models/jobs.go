package models

import (
	"fmt"
	"strconv"
)

type JobState int

const (
	JOB_STATE_NEW JobState = iota
	JOB_STATE_IN_PROGRESS
	JOB_STATE_FINISHED
	_maxEventid
)

var JobStateName = map[int]string{
	0: "NEW",
	1: "IN_PROGERSS",
	2: "FINISHED",
}

var JobStateValue = map[string]JobState{
	JobStateName[0]: JOB_STATE_NEW,
	JobStateName[1]: JOB_STATE_IN_PROGRESS,
	JobStateName[2]: JOB_STATE_FINISHED,
}

func (e JobState) String() string {
	s, ok := JobStateName[int(e)]
	if ok {
		return s
	}
	return strconv.Itoa(int(e))
}

// UnmarshalJSON unmarshals b into MatchEvent.
func (e *JobState) UnmarshalJSON(b []byte) error {
	// From json.Unmarshaler: By convention, to approximate the behavior of
	// Unmarshal itself, Unmarshalers implement UnmarshalJSON([]byte("null")) as
	// a no-op.
	if string(b) == "null" {
		return nil
	}
	if e == nil {
		return fmt.Errorf("nil receiver passed to UnmarshalJSON")
	}

	if ci, err := strconv.ParseUint(string(b), 10, 32); err == nil {
		if ci >= uint64(_maxEventid) {
			return fmt.Errorf("invalid code: %q", ci)
		}

		*e = JobState(ci)
		return nil
	}

	s := string(b)
	if len(s) > 0 && s[0] == '"' {
		s = s[1:]
	}
	if len(s) > 0 && s[len(s)-1] == '"' {
		s = s[:len(s)-1]
	}

	if ev, ok := JobStateValue[s]; ok {
		*e = ev
		return nil
	}
	return fmt.Errorf("invalid code: %q", string(b))
}
