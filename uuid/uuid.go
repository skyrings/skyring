// Copyright 2015 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package uuid

import (
	"crypto/rand"
	"fmt"
	"io"
)

type UUID [16]byte

func (uuid *UUID) String() string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

func (uuid *UUID) IsZero() bool {
	return uuid[0] == 0
}

func New() *UUID {
	uuid := new(UUID)

	n, err := io.ReadFull(rand.Reader, uuid[:])
	if err != nil {
		fmt.Println("random uuidgen failed")
	} else if n != len(uuid) {
		fmt.Println("read only", n, "expected", len(uuid))
	} else {
		// variant bits; see section 4.1.1
		uuid[8] = uuid[8]&^0xc0 | 0x80
		// version 4 (pseudo-random); see section 4.1.3
		uuid[6] = uuid[6]&^0xf0 | 0x40
	}

	return uuid
}

func Equal(uuid1 *UUID, uuid2 *UUID) bool {
	for i, v := range uuid1 {
		if v != uuid2[i] {
			return false
		}
	}

	return true
}
