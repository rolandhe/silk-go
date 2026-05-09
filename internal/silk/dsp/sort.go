package dsp

// InsertionSortIncreasing — SKP_Silk_insertion_sort_increasing.
// Sort the first K elements of a[] in increasing order; index[] receives
// the permutation (so a_sorted[i] == a_original[index[i]]). Then sweep the
// remaining L-K elements, inserting any that are smaller than a[K-1] into
// the sorted prefix.
//
// Translated from vendor/silk/src/SKP_Silk_sort.c.
func InsertionSortIncreasing(a []int32, index []int32, L, K int32) {
	for i := int32(0); i < K; i++ {
		index[i] = i
	}
	for i := int32(1); i < K; i++ {
		value := a[i]
		j := i - 1
		for j >= 0 && value < a[j] {
			a[j+1] = a[j]
			index[j+1] = index[j]
			j--
		}
		a[j+1] = value
		index[j+1] = i
	}
	for i := K; i < L; i++ {
		value := a[i]
		if value < a[K-1] {
			j := K - 2
			for j >= 0 && value < a[j] {
				a[j+1] = a[j]
				index[j+1] = index[j]
				j--
			}
			a[j+1] = value
			index[j+1] = i
		}
	}
}

// InsertionSortDecreasingInt16 — SKP_Silk_insertion_sort_decreasing_int16.
// Sort the first K elements of a[] in decreasing order; index[] receives
// the permutation.
//
// Translated from vendor/silk/src/SKP_Silk_sort.c.
func InsertionSortDecreasingInt16(a []int16, index []int32, L, K int32) {
	for i := int32(0); i < K; i++ {
		index[i] = i
	}
	for i := int32(1); i < K; i++ {
		value := a[i]
		j := i - 1
		for j >= 0 && value > a[j] {
			a[j+1] = a[j]
			index[j+1] = index[j]
			j--
		}
		a[j+1] = value
		index[j+1] = i
	}
	for i := K; i < L; i++ {
		value := a[i]
		if value > a[K-1] {
			j := K - 2
			for j >= 0 && value > a[j] {
				a[j+1] = a[j]
				index[j+1] = index[j]
				j--
			}
			a[j+1] = value
			index[j+1] = i
		}
	}
}

// InsertionSortIncreasingAllValues — SKP_Silk_insertion_sort_increasing_all_values.
// Sort all L elements in increasing order. No index permutation tracked.
//
// Translated from vendor/silk/src/SKP_Silk_sort.c.
func InsertionSortIncreasingAllValues(a []int32, L int32) {
	for i := int32(1); i < L; i++ {
		value := a[i]
		j := i - 1
		for j >= 0 && value < a[j] {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = value
	}
}
