package bugfixwithtests

func InRange(items []string, idx int) bool {
	return idx >= 0 && idx <= len(items)
}
