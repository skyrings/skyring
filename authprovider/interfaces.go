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
	"github.com/skyrings/skyring/models"
	"net/http"
)

const (
	Internal = iota
	External
)

const (
	CurrentUser = "me"
)

// The AuthBackend interface defines a set of methods an AuthBackend must
// implement.
type AuthBackend interface {
	SaveUser(u models.User) error
	User(username string) (user models.User, e error)
	Users() (users []models.User, e error)
	DeleteUser(username string) error
	Close()
}

type AuthInterface interface {
	Login(rw http.ResponseWriter, req *http.Request, username string, password string) error
	Logout(rw http.ResponseWriter, req *http.Request) error
	Authorize(rw http.ResponseWriter, req *http.Request) error
	AuthorizeRole(rw http.ResponseWriter, req *http.Request, role string) error
	AddUser(user models.User, password string) error
	UpdateUser(username string, m map[string]interface{}) error
	GetUser(username string, req *http.Request) (models.User, error)
	ListUsers() ([]models.User, error)
	DeleteUser(username string) error
	ListExternalUsers() ([]models.User, error)
}
