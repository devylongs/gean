package types

func CurrentSlot(genesisTime, currentTimeMs uint64) uint64 {
	genesisMs, ok := unixMillis(genesisTime)
	if !ok {
		return 0
	}
	if currentTimeMs < genesisMs {
		return 0
	}
	return (currentTimeMs - genesisMs) / MillisecondsPerSlot
}

// MillisIntoSlot returns how far the clock has advanced into the current slot.
func MillisIntoSlot(genesisTime, currentTimeMs uint64) uint64 {
	genesisMs, ok := unixMillis(genesisTime)
	if !ok {
		return 0
	}
	if currentTimeMs < genesisMs {
		return 0
	}
	return (currentTimeMs - genesisMs) % MillisecondsPerSlot
}

func CurrentInterval(genesisTime, currentTimeMs uint64) uint64 {
	return MillisIntoSlot(genesisTime, currentTimeMs) / MillisecondsPerInterval
}

func TotalIntervals(genesisTime, currentTimeMs uint64) uint64 {
	genesisMs, ok := unixMillis(genesisTime)
	if !ok {
		return 0
	}
	if currentTimeMs < genesisMs {
		return 0
	}
	return (currentTimeMs - genesisMs) / MillisecondsPerInterval
}

func IntervalsFromSlot(slot uint64) uint64 {
	if slot > ^uint64(0)/IntervalsPerSlot {
		return ^uint64(0)
	}
	return slot * IntervalsPerSlot
}

func IntervalsFromUnixTime(unixSeconds, genesisTime uint64) uint64 {
	if unixSeconds < genesisTime {
		return 0
	}
	return elapsedSecondsToIntervals(unixSeconds - genesisTime)
}

func unixMillis(seconds uint64) (uint64, bool) {
	if seconds > ^uint64(0)/1000 {
		return 0, false
	}
	return seconds * 1000, true
}

func elapsedSecondsToIntervals(seconds uint64) uint64 {
	wholeSlots := seconds / SecondsPerSlot
	remainder := seconds % SecondsPerSlot
	if wholeSlots > ^uint64(0)/IntervalsPerSlot {
		return ^uint64(0)
	}
	base := wholeSlots * IntervalsPerSlot
	extra := remainder * 1000 / MillisecondsPerInterval
	if base > ^uint64(0)-extra {
		return ^uint64(0)
	}
	return base + extra
}
