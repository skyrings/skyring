package util

import (
	"fmt"
	"reflect"
	"strings"
)

/*
Golang doesn't provide Set type.
The easiest O(1) way of implementing one is using maps.
*/
type Set map[interface{}]bool

var typ reflect.Type

//Use this for a type independent Set
func NewSet() Set {
	return make(map[interface{}]bool)
}

// Use this for conventional Set of a required type
func NewSetWithType(t reflect.Type) Set {
	typ = t
	return make(map[interface{}]bool)
}

func (s *Set) InsertAll(elements []interface{}) error {
	var errStr string
	for _, element := range elements {
		if err := s.Insert(element); err != nil {
			errStr = errStr + err.Error() + "\n"
		}
	}
	errStr = strings.TrimSpace(errStr)
	return fmt.Errorf(errStr)
}

func (s *Set) Insert(element interface{}) error {
	if typ != nil && reflect.TypeOf(element) != typ {
		return fmt.Errorf("Element not of desired type")
	}
	if (*s)[element] == false {
		(*s)[element] = true
		return nil
	}
	return fmt.Errorf("Element %v is duplicate", element)
}

func (s *Set) Delete(element interface{}) error {
	if (*s)[element] == false {
		return fmt.Errorf("Element %v not in set", element)
	}
	delete((*s), element)
	return nil
}

func (s Set) GetElements() (values []interface{}) {
	for key := range s {
		values = append(values, key)
	}
	return values
}
