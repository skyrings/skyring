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
	"errors"
	"fmt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/daos"
	"github.com/skyrings/skyring/dbprovider"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"gopkg.in/mgo.v2"
	"io"
	"time"
)

const (
	ProviderName = "mongodbprovider"
)

type MongoDb struct {
	Session *mgo.Session
}

func init() {
	dbprovider.RegisterDbProvider(ProviderName, func(config io.Reader) (dbprovider.DbInterface, error) {
		return NewMongoDbProvider(config)
	})
}

func NewMongoDbProvider(config io.Reader) (*MongoDb, error) {
	var (
		err     error
		mongoDb MongoDb
	)
	dbconf := conf.SystemConfig.DBConfig
	mongoDb.Session, err = mgo.DialWithInfo(&mgo.DialInfo{
		Addrs:    []string{fmt.Sprintf("%s:%d", dbconf.Hostname, dbconf.Port)},
		Timeout:  60 * time.Second,
		Database: dbconf.Database,
		Username: dbconf.User,
		Password: dbconf.Password,
	})
	if err != nil {
		logger.Get().Critical("Error: %v", err)
		return nil, err
	}
	return &mongoDb, nil
}

func (m MongoDb) Connect(document string) *mgo.Collection {
	session := m.Session.Copy()
	return session.DB(conf.SystemConfig.DBConfig.Database).C(document)
}

func (m MongoDb) Close(c *mgo.Collection) {
	c.Database.Session.Close()
}

func mkmgoerror(msg string) error {
	return errors.New(msg)
}

//Set up the indexes for the Db
//Can be called during the initialization
func (m MongoDb) InitDb() error {

	//Set the indexes for storage profiles
	c := m.Connect(models.COLL_NAME_USER)
	defer c.Database.Session.Close()

	index := mgo.Index{
		Key:    []string{"username"},
		Unique: true,
	}
	err := c.EnsureIndex(index)
	if err != nil {
		logger.Get().Error("Error Setting goauth collection:%s", err)
		return mkmgoerror(err.Error())
	}
	return err
}

func (m MongoDb) UserInterface() dao.UserInterface {
	return m
}
