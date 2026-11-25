package link

// containsID reports whether the edge list already holds id.
func containsID(values []string, id string) bool {
	for _, v := range values {
		if v == id {
			return true
		}
	}
	return false
}

// addUnique appends id when absent, leaving ordering untouched so an existing file's list order is preserved.
func addUnique(values []string, id string) ([]string, bool) {
	if containsID(values, id) {
		return values, false
	}
	return append(values, id), true
}

// removeID drops id when present.
func removeID(values []string, id string) ([]string, bool) {
	out := values[:0:0]
	changed := false
	for _, v := range values {
		if v == id {
			changed = true
			continue
		}
		out = append(out, v)
	}
	return out, changed
}
