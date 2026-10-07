package core

func calculateContactsHash(ids []int64) int64 {
	const hashMod = uint64(0x80000000)
	var hash uint64
	for _, id := range ids {
		hash = (hash*20261 + hashMod + uint64(id)) % hashMod
	}
	return int64(hash)
}
