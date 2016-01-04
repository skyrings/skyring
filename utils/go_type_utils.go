package util

import (
	"fmt"
	"reflect"
)

func GetMapKeys(inMap interface{}) ([]reflect.Value, error) {
	inMapValue := reflect.ValueOf(inMap)
	if inMapValue.Kind() != reflect.Map {
		return nil, fmt.Errorf("Not a map!!")
	}
	keys := inMapValue.MapKeys()
	return keys, nil
}

func Stringify(keys []reflect.Value) (strings []string) {
	strings = make([]string, len(keys))
	for index, key := range keys {
		strings[index] = key.String()
	}
	return strings
}

func StringSetDiff(keys1 []string, keys2 []string) (diff []string) {
	var unique bool
	for _, key1 := range keys1 {
		unique = true
		for _, key2 := range keys2 {
			if key1 == key2 {
				unique = false
				continue
			}
		}
		if unique {
			diff = append(diff, key1)
		}
	}
	return diff
}
