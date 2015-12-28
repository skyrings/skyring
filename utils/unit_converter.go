/*Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

type Unit struct {
	Name             string
	NoOfPrevUnitEqui int
	Order            int
}

type Units []Unit

func (slice Units) Len() int {
	return len(slice)
}

func (slice Units) Less(i, j int) bool {
	return slice[i].Order < slice[j].Order
}

func (slice Units) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func GetSizeUnits() Units {
	return Units{
		{
			Name:             "Bytes",
			NoOfPrevUnitEqui: 1,
			Order:            1,
		},
		{
			Name:             "KibiBytes",
			NoOfPrevUnitEqui: 1024,
			Order:            2,
		},
		{
			Name:             "MibiBytes",
			NoOfPrevUnitEqui: 1024,
			Order:            3,
		},
		{
			Name:             "GibiBytes",
			NoOfPrevUnitEqui: 1024,
			Order:            4,
		},
		{
			Name:             "TibiBytes",
			NoOfPrevUnitEqui: 1024,
			Order:            5,
		},
	}
}

func MakeUnits(orderedUnitNames []string, commonScale int) (units Units) {
	for index, unitName := range orderedUnitNames {
		noOfPrevUnitEqui := commonScale
		if index == 0 {
			noOfPrevUnitEqui = 1
		}
		units = append(units, Unit{Name: unitName, NoOfPrevUnitEqui: noOfPrevUnitEqui, Order: index + 1})
	}
	return units
}

var SizeUnits Units = MakeUnits([]string{"Bytes", "KibiBytes", "MibiBytes", "GibiBytes", "TibiBytes"}, 1024)

func (fromUnit Unit) Convert(fromValue float64, toUnit Unit, unitSet Units) (size float64) {
	size = fromValue
	loopOk := fromUnit.Order != toUnit.Order
	index := fromUnit.Order
	for loopOk {
		if fromUnit.Order < toUnit.Order {
			size = size / (float64(unitSet[index].NoOfPrevUnitEqui))
			index++
		} else {
			index--
			size = size * (float64(unitSet[index].NoOfPrevUnitEqui))
		}
		if toUnit.Order == index {
			loopOk = false
		}
	}
	return size
}

func (fromUnit Unit) AutoConvert(fromValue float64, unitSet Units) (size float64, unit Unit) {
	size = fromValue
	index := fromUnit.Order - 1
	currentUnit := fromUnit
	if index != len(unitSet)-1 {
		for {
			index = index + 1
			size = currentUnit.Convert(size, unitSet[index], unitSet)
			currentUnit = unitSet[index]
			if index == len(unitSet)-1 || size < float64(unitSet[index+1].NoOfPrevUnitEqui) {
				return size, unitSet[index]
			}
		}
	}
	return size, unitSet[index]
}
