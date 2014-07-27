package shapeset

import (
	"errors"
	"strconv"
	"strings"
)

// Parses a string of comma seperated floats to produce a slice of floats
func parseCSFloats(csfloats string) (floats []float64, err error) {
	string_segments := strings.Split(csfloats, ",")
	floats = make([]float64, 0, len(string_segments))
	var num float64
	for _, seg := range string_segments {
		num, err = strconv.ParseFloat(seg, 64)
		if err != nil {
			err = errors.New("Could not parse float64 from: " + seg)
			return
		}
		floats = append(floats, num)
	}
	return
}

// Parses a string of comma seperated ints to produce a slice of ints
func parseCSInts(csints string) (ints []int, err error) {
	string_segments := strings.Split(csints, ",")
	ints = make([]int, 0, len(string_segments))
	var num int
	for _, seg := range string_segments {
		num, err = strconv.Atoi(seg)
		if err != nil {
			err = errors.New("Could not parse int from: " + seg)
			return
		}
		ints = append(ints, num)
	}
	return
}

// Checks if the given string appears in the given slice of strings
func stringInSlice(s string, strs []string) bool {
	for _, str := range strs {
		if s == str {
			return true
		}
	}
	return false
}

// Checks if the given int appears in the given slice of ints
func intInSlice(s int, ints []int) bool {
	for _, interger := range ints {
		if s == interger {
			return true
		}
	}
	return false
}
