package ygopro_data

import (
	"strings"
	"sort"
)

type Set struct {
	Locale     string
	Name       string
	Code       int64
	Ids        []int
	OriginName string
}

func createSet(code int64, name string, locale string) Set {
	set := Set{locale, name, code, make([]int, 0), ""}
	set.separateOriginNameFromName()
	return set
}

func (set *Set) separateOriginNameFromName() bool {
	names := strings.Split(set.Name, "\t")
	if len(names) <= 1 {
		return false
	} else {
		set.Name = names[0]
		set.OriginName = names[1]
		return true
	}
}

func (set *Set) includes(id int) bool {
	for _, containedId := range set.Ids {
		if id == containedId {
			return true
		}
	}
	return false
}

func (set *Set) includeInSort(id int) bool {
	low := 0
	high := len(set.Ids) - 1
	for ; low <= high; {
		mid := (low + high) / 2
		if set.Ids[mid] == id {
			return true
		} else if set.Ids[mid] < id {
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	return false
}

func (set *Set) Sort() {
	sort.Ints(set.Ids)
}