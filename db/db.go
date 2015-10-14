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
package db

import (
	"fmt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/tools/logger"
	"gopkg.in/mgo.v2"
	"net/url"
	"time"

	influxdb "github.com/influxdb/influxdb/client"
)

var log = logger.Get()

var (
	session        *mgo.Session
	influxdbClient *influxdb.Client
)

func InitDBSession(dbconf conf.MongoDBConfig) error {
	var err error
	session, err = mgo.DialWithInfo(&mgo.DialInfo{
		Addrs:    []string{fmt.Sprintf("%s:%d", dbconf.Hostname, dbconf.Port)},
		Timeout:  60 * time.Second,
		Database: dbconf.Database,
		Username: dbconf.User,
		Password: dbconf.Password,
	})
	if err != nil {
		log.Critical("Error: %v", err)
		return err
	}
	return nil
}

func GetDatastore() *mgo.Session {
	return session
}

func InitMonitoringDB(mondbconf conf.InfluxDBconfig) error {
	u, err := url.Parse(fmt.Sprintf("http://%s:%d",
		mondbconf.Hostname, mondbconf.Port))
	if err != nil {
		log.Critical("Error: %v", err)
		return err
	}

	influxdbClient, err = influxdb.NewClient(influxdb.Config{
		URL:      *u,
		Username: mondbconf.User,
		Password: mondbconf.Password,
	})
	if err != nil {
		log.Critical("Error: %v", err)
		return err
	}

	return nil
}

func GetMonitoringDBClient() *influxdb.Client {
	return influxdbClient
}
