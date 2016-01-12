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
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/utils"
	"golang.org/x/crypto/bcrypt"
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
		logger.Get().Error("Unable to create default User:%s", err)
		return err
	}

	return nil
}

func (a *App) login(rw http.ResponseWriter, req *http.Request) {

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("Error parsing http request body:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	if err := GetAuthProvider().Login(rw, req, m["username"].(string), m["password"].(string)); err != nil {
		logger.Get().Error("Unable to login User:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	bytes, _ := json.Marshal(`{'message': 'Logged in'}`)
	rw.Write(bytes)
}

func (a *App) logout(rw http.ResponseWriter, req *http.Request) {
	if err := GetAuthProvider().Logout(rw, req); err != nil {
		logger.Get().Error("Unable to logout User:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	bytes, _ := json.Marshal(`{'message': 'Logged out'}`)
	rw.Write(bytes)
}

func (a *App) getUsers(rw http.ResponseWriter, req *http.Request) {

	users, err := GetAuthProvider().ListUsers()
	if err != nil {
		logger.Get().Error("Unable to List the users:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	//hide the hash field for users in the list
	type PublicUser struct {
		*models.User
		Hash bool `json:"hash,omitempty"`
	}
	var pUsers []PublicUser
	for _, user := range users {

		pUsers = append(pUsers, PublicUser{
			User: &models.User{Username: user.Username,
				Email:               user.Email,
				Role:                user.Role,
				Groups:              user.Groups,
				Type:                user.Type,
				Status:              user.Status,
				FirstName:           user.FirstName,
				LastName:            user.LastName,
				NotificationEnabled: user.NotificationEnabled},
		})
	}
	//marshal and send it across
	bytes, err := json.Marshal(pUsers)
	if err != nil {
		logger.Get().Error("Unable to marshal the list of Users:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}

func (a *App) getUser(rw http.ResponseWriter, req *http.Request) {

	vars := mux.Vars(req)

	user, err := GetAuthProvider().GetUser(vars["username"], req)
	if err != nil {
		logger.Get().Error("Unable to Get the user:%s", err)
		util.HandleHttpError(rw, err)
		return
	}

	//hide the hash field
	bytes, err := json.Marshal(struct {
		*models.User
		Hash bool `json:"hash,omitempty"`
	}{
		User: &user,
	})
	if err != nil {
		logger.Get().Error("Unable to marshal the User:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}

func (a *App) getExternalUsers(rw http.ResponseWriter, req *http.Request) {

	users, err := GetAuthProvider().ListExternalUsers()
	if err != nil {
		logger.Get().Error("Unable to List the users:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	//hide the hash field for users in the list
	type PublicUser struct {
		*models.User
		Hash         bool `json:"hash,omitempty"`
		Notification bool `json:"notification,omitempty"`
	}
	var pUsers []PublicUser
	for _, user := range users {

		pUsers = append(pUsers, PublicUser{
			User: &models.User{Username: user.Username,
				Email:               user.Email,
				Role:                user.Role,
				Groups:              user.Groups,
				Type:                user.Type,
				Status:              user.Status,
				FirstName:           user.FirstName,
				LastName:            user.LastName,
				NotificationEnabled: user.NotificationEnabled},
		})
	}
	//marshal and send it across
	bytes, err := json.Marshal(pUsers)
	if err != nil {
		logger.Get().Error("Unable to marshal the list of Users:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)
}

func (a *App) addUsers(rw http.ResponseWriter, req *http.Request) {

	var user models.User

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("Error parsing http request body:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	if val, ok := m["username"]; ok {
		user.Username = val.(string)
	}
	if val, ok := m["email"]; ok {
		user.Email = val.(string)
	}
	if val, ok := m["role"]; ok {
		user.Role = val.(string)
	}
	if val, ok := m["type"]; ok {
		user.Type = int(val.(float64))
	}
	if val, ok := m["firstname"]; ok {
		user.FirstName = val.(string)
	}
	if val, ok := m["lastname"]; ok {
		user.LastName = val.(string)
	}
	if val, ok := m["notificationenabled"]; ok {
		user.NotificationEnabled = val.(bool)
	}

	if err := GetAuthProvider().AddUser(user, m["password"].(string)); err != nil {
		logger.Get().Error("Unable to create User:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
}

func (a *App) modifyUsers(rw http.ResponseWriter, req *http.Request) {

	vars := mux.Vars(req)
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("Error parsing http request body:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		util.HandleHttpError(rw, err)
		return
	}

	user, err := GetAuthProvider().GetUser(vars["username"], req)
	if err != nil {
		logger.Get().Error("Unable to Get the user:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	verify := bcrypt.CompareHashAndPassword(user.Hash, []byte(m["oldpassword"].(string)))
	if verify != nil {
		logger.Get().Error("Old password is incorrect")
		bytes, _ := json.Marshal(`{'message': 'Old password is incorrect'}`)
		rw.Write(bytes)
	} else {
		if err := GetAuthProvider().UpdateUser(vars["username"], m); err != nil {
			logger.Get().Error("Unable to update User:%s", err)
			util.HandleHttpError(rw, err)
			return
		}
		bytes, _ := json.Marshal(`{'message': 'password-changed'}`)
		rw.Write(bytes)
	}
}

func (a *App) deleteUser(rw http.ResponseWriter, req *http.Request) {

	vars := mux.Vars(req)

	if err := GetAuthProvider().DeleteUser(vars["username"]); err != nil {
		logger.Get().Error("Unable to delete User:%s", err)
		util.HandleHttpError(rw, err)
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
		logger.Get().Error("Error parsing http request body:%s", err)
		return errors.New(err.Error())
	}
	if err = json.Unmarshal(body, &user); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		return errors.New(err.Error())
	}
	return nil
}
