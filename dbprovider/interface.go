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
package dbprovider

import (
	//"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/models"
)

type DbInterface interface {
	InitDb() error

	//User Model
	User(username string) (user models.User, e error)
	Users() (users []models.User, e error)
	SaveUser(u models.User) error
	DeleteUser(username string) error
	//StorageProfile
	StorageProfile(name string) (sProfile models.StorageProfile, e error)
	StorageProfiles() (sProfiles []models.StorageProfile, e error)
	SaveStorageProfile(s models.StorageProfile) error
	DeleteStorageProfile(name string) error
}
