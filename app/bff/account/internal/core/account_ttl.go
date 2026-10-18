package core

func validAccountTTLDays(days int32) bool {
	switch days {
	case 30, 90, 180, 182, 183, 365, 548, 730:
		return true
	default:
		return false
	}
}
