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

package localauthprovider

import (
	"errors"
	"github.com/skyrings/skyring/authprovider"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// MongodbAuthBackend stores database connection information.
type MongodbAuthBackend struct {
	mongoURL string
	database string
	session  *mgo.Session
}

func (b MongodbAuthBackend) connect() *mgo.Collection {
	session := b.session.Copy()
	return session.DB(b.database).C("goauth")
}

func mkmgoerror(msg string) error {
	return errors.New("mongobackend: " + msg)
}

// NewMongodbBackend initializes a new backend.
// Be sure to call Close() on this to clean up the mongodb connection.
// Example:
//     backend = httpauth.MongodbAuthBackend("mongodb://127.0.0.1/", "auth")
//     defer backend.Close()
func NewMongodbBackend(mongoURL string, database string) (b MongodbAuthBackend, e error) {
	// Set up connection to database
	b.mongoURL = mongoURL
	b.database = database
	session, err := mgo.Dial(b.mongoURL)
	if err != nil {
		return b, mkmgoerror(err.Error())
	}
	err = session.Ping()
	if err != nil {
		return b, mkmgoerror(err.Error())
	}

	// Ensure that the Username field is unique
	index := mgo.Index{
		Key:    []string{"Username"},
		Unique: true,
	}
	err = session.DB(b.database).C("goauth").EnsureIndex(index)
	if err != nil {
		return b, mkmgoerror(err.Error())
	}
	b.session = session
	return
}

// User returns the user with the given username. Error is set to
// ErrMissingUser if user is not found.
func (b MongodbAuthBackend) User(username string) (user authprovider.User, e error) {
	var result authprovider.User

	c := b.connect()
	defer c.Database.Session.Close()

	err := c.Find(bson.M{"Username": username}).One(&result)
	if err != nil {
		return result, ErrMissingUser
	}
	return result, nil
}

// Users returns a slice of all users.
func (b MongodbAuthBackend) Users() (us []authprovider.User, e error) {
	c := b.connect()
	defer c.Database.Session.Close()

	err := c.Find(bson.M{}).All(&us)
	if err != nil {
		return us, mkmgoerror(err.Error())
	}
	return
}

// SaveUser adds a new user, replacing if the same username is in use.
func (b MongodbAuthBackend) SaveUser(user authprovider.User) error {
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
		return ErrDeleteNull
	}
	return err
}

// Close cleans up the backend once done with. This should be called before
// program exit.
func (b MongodbAuthBackend) Close() {
	if b.session != nil {
		b.session.Close()
	}
}
