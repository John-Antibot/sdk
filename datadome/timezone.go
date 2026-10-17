package datadome

import "time"

// JSOffsetMinutes returns JavaScript's Date.getTimezoneOffset() for zone at time at.
func JSOffsetMinutes(zone string, at time.Time) (int, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return 0, err
	}
	_, offsetSeconds := at.In(loc).Zone()
	return -offsetSeconds / 60, nil
}
