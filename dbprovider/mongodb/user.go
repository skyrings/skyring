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
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrMissingUser = mkmgoerror("can't find user")
)

// User returns the user with the given username. Error is set to
// ErrMissingUser if user is not found.
func (m MongoDb) User(username string) (user models.User, e error) {
	c := m.Connect(models.COLL_NAME_USER)
	defer m.Close(c)
	err := c.Find(bson.M{"username": username}).One(&user)
	if err != nil {
		logger.Get().Error("Error getting record from DB:%s", err)
		return user, ErrMissingUser
	}
	return user, nil
}

// Users returns a slice of all users.
func (m MongoDb) Users() (us []models.User, e error) {
	c := m.Connect(models.COLL_NAME_USER)
	defer m.Close(c)

	err := c.Find(bson.M{}).All(&us)
	if err != nil {
		logger.Get().Error("Error getting record from DB:%s", err)
		return us, mkmgoerror(err.Error())
	}
	return us, nil
}

// SaveUser adds a new user, replacing if the same username is in use.
func (m MongoDb) SaveUser(user models.User) error {
	c := m.Connect(models.COLL_NAME_USER)
	defer m.Close(c)

	_, err := c.Upsert(bson.M{"username": user.Username}, bson.M{"$set": user})
	if err != nil {
		logger.Get().Error("Error deleting record from DB:%s", err)
		return mkmgoerror(err.Error())
	}
	return nil
}

// DeleteUser removes a user. ErrNotFound is returned if the user isn't found.
func (m MongoDb) DeleteUser(username string) error {
	c := m.Connect(models.COLL_NAME_USER)
	defer m.Close(c)

	// raises error if "username" doesn't exist
	err := c.Remove(bson.M{"username": username})
	if err != nil {
		logger.Get().Error("Error deleting record from DB:%s", err)
		return mkmgoerror(err.Error())
	}
	return err
}

func (m MongoDb) InitUser() error {
	//Set the indexes for User
	c := m.Connect(models.COLL_NAME_USER)
	defer c.Database.Session.Close()

	index := mgo.Index{
		Key:    []string{"username"},
		Unique: true,
	}
	err := c.EnsureIndex(index)
	if err != nil {
		logger.Get().Error("Error Setting the Index:%s", err)
		return mkmgoerror(err.Error())
	}
	return err
}
