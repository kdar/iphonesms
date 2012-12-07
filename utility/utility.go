package utility

func Contains(element int, slice []int) bool {
	return Index(element, slice) != -1
}

func Index(element int, slice []int) int {
	for i, x := range slice {
		if x == element {
			return i
		}
	}

	return -1
}
