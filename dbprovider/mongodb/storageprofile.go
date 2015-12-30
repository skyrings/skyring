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
package mongodb

import (
	"fmt"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func (m MongoDb) StorageProfile(name string) (sProfile models.StorageProfile, e error) {

	c := m.Connect(models.COLL_NAME_STORAGE_PROFILE)
	defer m.Close(c)
	fmt.Println(name)
	err := c.Find(bson.M{"name": name}).One(&sProfile)
	if err != nil {
		if err == mgo.ErrNotFound {
			fmt.Println("Yes It is")
		}
		logger.Get().Error("Error getting record from DB:%s", err)
		fmt.Println(err)
		return sProfile, mkmgoerror(err.Error())
	}
	fmt.Println(sProfile)
	return sProfile, nil
}

func (m MongoDb) StorageProfiles() (sProfiles []models.StorageProfile, e error) {

	c := m.Connect(models.COLL_NAME_STORAGE_PROFILE)
	defer m.Close(c)

	err := c.Find(bson.M{}).All(&sProfiles)
	if err != nil {
		logger.Get().Error("Error getting record from DB:%s", err)
		return sProfiles, mkmgoerror(err.Error())
	}
	return sProfiles, nil

}
func (m MongoDb) SaveStorageProfile(s models.StorageProfile) error {
	c := m.Connect(models.COLL_NAME_STORAGE_PROFILE)
	defer m.Close(c)

	_, err := c.Upsert(bson.M{"name": s.Name}, bson.M{"$set": s})
	if err != nil {
		logger.Get().Error("Error deleting record from DB:%s", err)
		return mkmgoerror(err.Error())
	}
	return nil

}
func (m MongoDb) DeleteStorageProfile(name string) error {
	c := m.Connect(models.COLL_NAME_STORAGE_PROFILE)
	defer m.Close(c)

	err := c.Remove(bson.M{"name": name})
	if err != nil {
		logger.Get().Error("Error deleting record from DB:%s", err)
		return mkmgoerror(err.Error())
	}
	return nil
}
