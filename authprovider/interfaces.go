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
	"net/http"
)

type User struct {
	Username string   `bson:"Username"`
	Email    string   `bson:"Email"`
	Hash     []byte   `bson:"Hash"`
	Role     string   `bson:"Role"`
	Groups   []string `bson:"Groups"`
}

type AuthInterface interface {
	Login(rw http.ResponseWriter, req *http.Request, u string, p string) error
	Logout(rw http.ResponseWriter, req *http.Request) error
	Authorize(rw http.ResponseWriter, req *http.Request) error
	AuthorizeRole(rw http.ResponseWriter, req *http.Request, role string) error
	AddUser(user User, password string) error
	UpdateUser(req *http.Request, password string, email string) error
	CurrentUser(rw http.ResponseWriter, req *http.Request) (user User, err error)
	DeleteUser(username string) error
	ListUsers() (users []User, err error)
}
