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
    "gopkg.in/mgo.v2"
    "time"
)

const (
    MongoDBHost = "127.0.0.1:27017"
    AuthDatabase = "skyring"
    AuthUserName = "admin"
    AuthPassword = "admin"
    AppDatabase = "skyring"
)

var (
    session, err = mgo.DialWithInfo(&mgo.DialInfo{
            Addrs: []string{MongoDBHost},
            Timeout: 60 * time.Second,
            Database: AppDatabase,
            Username: AuthUserName,
            Password: AuthPassword,
            })
)

func GetDatastore() *mgo.Session {
    return session
}
