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

package authprovider

import (
	"errors"
	"github.com/golang/glog"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrDeleteNull  = mkmgoerror("deleting non-existant user")
	ErrMissingUser = mkmgoerror("can't find user")
)

// MongodbAuthBackend stores database connection information.
type MongodbAuthBackend struct {
}

func (b MongodbAuthBackend) connect() *mgo.Collection {
	session := db.GetDatastore().Copy()
	return session.DB(conf.SystemConfig.DBConfig.Database).C("goauth")
}

func mkmgoerror(msg string) error {
	return errors.New(msg)
}

// NewMongodbBackend initializes a new backend.
// Be sure to call Close() on this to clean up the mongodb connection.
// Example:
//     backend = httpauth.MongodbAuthBackend("mongodb://127.0.0.1/", "auth")
//     defer backend.Close()
func NewMongodbBackend() (b MongodbAuthBackend, e error) {

	// Ensure that the Username field is unique
	index := mgo.Index{
		Key:    []string{"Username"},
		Unique: true,
	}

	c := b.connect()
	defer c.Database.Session.Close()

	err := c.EnsureIndex(index)
	if err != nil {
		glog.Errorf("Error Setting goauth collection:%s", err)
		return b, mkmgoerror(err.Error())
	}
	return
}

// User returns the user with the given username. Error is set to
// ErrMissingUser if user is not found.
func (b MongodbAuthBackend) User(username string) (user models.User, e error) {
	c := b.connect()
	defer c.Database.Session.Close()

	err := c.Find(bson.M{"Username": username}).One(&user)
	if err != nil {
		glog.Errorf("Error getting record from DB:%s", err)
		return user, ErrMissingUser
	}
	return user, nil
}

// Users returns a slice of all users.
func (b MongodbAuthBackend) Users() (us []models.User, e error) {
	c := b.connect()
	defer c.Database.Session.Close()

	err := c.Find(bson.M{}).All(&us)
	if err != nil {
		glog.Errorf("Error getting record from DB:%s", err)
		return us, mkmgoerror(err.Error())
	}
	return
}

// SaveUser adds a new user, replacing if the same username is in use.
func (b MongodbAuthBackend) SaveUser(user models.User) error {
	c := b.connect()
	defer c.Database.Session.Close()

	_, err := c.Upsert(bson.M{"Username": user.Username}, bson.M{"$set": user})
	return err
}

// DeleteUser removes a user. ErrNotFound is returned if the user isn't found.
func (b MongodbAuthBackend) DeleteUser(username string) error {
	c := b.connect()
	defer c.Database.Session.Close()

	// raises error if "username" doesn't exist
	err := c.Remove(bson.M{"Username": username})
	if err == mgo.ErrNotFound {
		glog.Errorf("Error deleting record from DB:%s", err)
		return ErrDeleteNull
	}
	return err
}

// Close cleans up the backend once done with. This should be called before
// program exit.
func (b MongodbAuthBackend) Close() {
	//No need to do anything as the session is maintained by the db package
}
