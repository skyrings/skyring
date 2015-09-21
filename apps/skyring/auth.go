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
package skyring

import (
	"encoding/json"
	"errors"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/utils"
	"io/ioutil"
	"net/http"
)

const (
	DefaultUserName = "admin"
	DefaultPassword = "admin"
	DefaultEmail    = "admin@localhost"
	DefaultRole     = "admin"
)

func AddDefaultUser() error {

	defaultUser := models.User{Username: DefaultUserName, Email: DefaultEmail, Role: DefaultRole}

	if err := GetAuthProvider().AddUser(defaultUser, DefaultPassword); err != nil {
		glog.Errorf("Unable to create default User:%s", err)
		return err
	}

	return nil
}

func login(rw http.ResponseWriter, req *http.Request) {

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		glog.Errorf("Error parsing http request body:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(body, &m); err != nil {
		glog.Errorf("Unable to Unmarshall the data:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	if err := GetAuthProvider().Login(rw, req, m["username"].(string), m["password"].(string)); err != nil {
		glog.Errorf("Unable to login User:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	bytes, _ := json.Marshal(`{'message': 'Logged in'}`)
	rw.Write(bytes)
}

func logout(rw http.ResponseWriter, req *http.Request) {
	if err := GetAuthProvider().Logout(rw, req); err != nil {
		glog.Errorf("Unable to logout User:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	bytes, _ := json.Marshal(`{'message': 'Logged out'}`)
	rw.Write(bytes)
}

func getUsers(rw http.ResponseWriter, req *http.Request) {

	users, err := GetAuthProvider().ListUsers()
	if err != nil {
		glog.Errorf("Unable to List the users:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	//hide the hash field for users in the list
	type PublicUser struct {
		*models.User
		Hash bool `json:"Hash,omitempty"`
	}
	var pUsers []PublicUser
	for _, user := range users {

		pUsers = append(pUsers, PublicUser{
			User: &user,
		})
	}
	//marshal and send it across
	bytes, err := json.Marshal(pUsers)
	if err != nil {
		glog.Errorf("Unable to marshal the list of Users:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}

func getUser(rw http.ResponseWriter, req *http.Request) {

	vars := mux.Vars(req)

	user, err := GetAuthProvider().GetUser(vars["username"])
	if err != nil {
		glog.Errorf("Unable to Get the user:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}

	//hide the hash field
	bytes, err := json.Marshal(struct {
		*models.User
		Hash bool `json:"Hash,omitempty"`
	}{
		User: &user,
	})
	if err != nil {
		glog.Errorf("Unable to marshal the User:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}

func addUsers(rw http.ResponseWriter, req *http.Request) {

	var user models.User

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		glog.Errorf("Error parsing http request body:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(body, &m); err != nil {
		glog.Errorf("Unable to Unmarshall the data:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}

	user.Username = m["username"].(string)
	user.Email = m["email"].(string)
	user.Role = m["role"].(string)

	if err := GetAuthProvider().AddUser(user, m["password"].(string)); err != nil {
		glog.Errorf("Unable to create User:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
}

func deleteUsers(rw http.ResponseWriter, req *http.Request) {

	var user models.User

	//Get the request details from requestbody

	if err := parseAuthRequestBody(req, &user); err != nil {
		glog.Errorf("Unable to parse the request data:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	if err := GetAuthProvider().DeleteUser(user.Username); err != nil {
		glog.Errorf("Unable to delete User:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
}

func parseAuthRequestBody(req *http.Request, user *models.User) error {
	//Get the request details from requestbody
	var (
		body []byte
		err  error
	)
	if body, err = ioutil.ReadAll(req.Body); err != nil {
		glog.Errorf("Error parsing http request body:%s", err)
		return errors.New(err.Error())
	}
	if err = json.Unmarshal(body, &user); err != nil {
		glog.Errorf("Unable to Unmarshall the data:%s", err)
		return errors.New(err.Error())
	}
	return nil
}
