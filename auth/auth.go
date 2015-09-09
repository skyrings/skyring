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
package auth

import (
	"encoding/json"
	"errors"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/trusch/httpauth"
	"golang.org/x/crypto/bcrypt"
	"io/ioutil"
	"net/http"
	"skyring/models"
	"skyring/utils"
)

var (
	Backend        httpauth.MongodbAuthBackend
	HttpAuthorizer httpauth.Authorizer
	Roles          map[string]httpauth.Role
)

type User model.User

func InitAuthorizer() error {
	glog.Infof("Inside InitAuth")
	//TODO make the db and server URL configurable
	var err error
	Backend, err = httpauth.NewMongodbBackend("127.0.0.1", "test")
	if err != nil {
		glog.Errorf("unable to create Mongodb backend", err)
	}
	// create some default roles
	//TODO User should be able to create roles and assign roles, will be done later
	Roles = make(map[string]httpauth.Role)
	Roles["user"] = 30
	Roles["admin"] = 80

	//create the authorizer
	HttpAuthorizer, err = httpauth.NewAuthorizer(Backend, []byte("cookie-encryption-key"), "user", Roles)
	return err
}

func AddDefaultUser() error {
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		glog.Errorf("Unable to hash the password", err)
	}
	defaultUser := httpauth.UserData{Username: "admin", Email: "admin@localhost", Hash: hash, Role: "admin"}
	err = Backend.SaveUser(defaultUser)
	if err != nil {
		glog.Errorf("Unable to create the default user", err)
	}
	return nil
}

func SetAuthRoutes(container *mux.Router) error {
	container.
		Methods("POST").
		Path("/login").
		Name("login").
		Handler(http.HandlerFunc(loginHandler))
	container.
		Methods("POST").
		Path("/logout").
		Name("logout").
		Handler(http.HandlerFunc(logoutHandler))
	container.
		Methods("GET").
		Path("/users").
		Name("getUsers").
		Handler(http.HandlerFunc(getUsersHandler))
	container.
		Methods("POST").
		Path("/users").
		Name("addUsers").
		Handler(http.HandlerFunc(addUsersHandler))
	container.
		Methods("DELETE").
		Path("/users").
		Name("deleteUsers").
		Handler(http.HandlerFunc(deleteUsersHandler))
	return nil
}

func loginHandler(rw http.ResponseWriter, req *http.Request) {
	glog.Infof("Inside Login Hander")
	var user User
	//Get the request details from requestbody
	if err := parseAuthRequestBody(req, &user); err != nil {
		glog.Errorf("Unable to parse the request data", err)
		util.HandleHHttpError(rw, err)
		return
	}
	if err := HttpAuthorizer.Login(rw, req, user.Username, user.Password, "/"); err != nil && err.Error() == "httpauth: already authenticated" {
		glog.Errorf("Unable to login User", err)
		bytes, _ := json.Marshal(util.APIError{Error: err.Error()})
		rw.Write(bytes)
		return
	} else if err != nil {
		glog.Errorf("Unable to login User", err)
		util.HandleHHttpError(rw, err)
		return
	}
	glog.Infof("Out Login Hander")
	//rw.WriteHeader(http.StatusOK)
}
func logoutHandler(rw http.ResponseWriter, req *http.Request) {
	glog.Infof("Inside Logout Hander")
	if err := HttpAuthorizer.Logout(rw, req); err != nil {
		glog.Errorf("Unable to logout User", err)
		util.HandleHHttpError(rw, err)
		return
	}
	//rw.WriteHeader(http.StatusOK)
}
func getUsersHandler(rw http.ResponseWriter, req *http.Request) {
	glog.Infof("Inside getUsersHandler")
	var (
		users []httpauth.UserData
		bytes []byte
		err   error
	)
	if users, err = Backend.Users(); err != nil {
		glog.Errorf("Unable get the listeHHttpError of Users", err)
		util.HandleHHttpError(rw, err)
		return
	}
	if bytes, err = json.Marshal(users); err != nil {
		glog.Errorf("Unable marshal the list of Users", err)
		util.HandleHHttpError(rw, err)
		return
	}
	//rw.WriteHeader(http.StatusOK)
	rw.Write(bytes)

}
func addUsersHandler(rw http.ResponseWriter, req *http.Request) {
	glog.Infof("Inside addUsersHandler")
	var (
		userdata httpauth.UserData
		user     User
	)
	//Get the request details from requestbody
	if err := parseAuthRequestBody(req, &user); err != nil {
		glog.Errorf("Unable to parse the request data", err)
		util.HandleHHttpError(rw, err)
		return
	}
	userdata.Username = user.Username
	userdata.Email = user.Email
	userdata.Role = user.Role

	if err := HttpAuthorizer.Register(rw, req, userdata, user.Password); err != nil {
		glog.Errorf("Unable to create User", err)
		util.HandleHHttpError(rw, err)
		return
	}
	//rw.WriteHeader(http.StatusOK)
}
func deleteUsersHandler(rw http.ResponseWriter, req *http.Request) {
	glog.Infof("Inside addUsersHandler")
	var user User

	//Get the request details from requestbody

	if err := parseAuthRequestBody(req, &user); err != nil {
		glog.Errorf("Unable to parse the request data", err)
		util.HandleHHttpError(rw, err)
		return
	}
	if err := HttpAuthorizer.DeleteUser(user.Username); err != nil {
		glog.Errorf("Unable to delete User", err)
		util.HandleHHttpError(rw, err)
		return
	}
	//rw.WriteHeader(http.StatusOK)
}

func parseAuthRequestBody(req *http.Request, user *User) error {
	//Get the request details from requestbody
	var (
		body []byte
		err  error
	)
	if body, err = ioutil.ReadAll(req.Body); err != nil {
		glog.Errorf("Error parsing http request body", err)
		return errors.New(err.Error())
	}
	if err = json.Unmarshal(body, &user); err != nil {
		glog.Errorf("Unable to Unmarshall the data", err)
		return errors.New(err.Error())
	}
	return nil
}
