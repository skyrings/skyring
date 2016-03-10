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
	"fmt"
	"github.com/gorilla/mux"
	common_event "github.com/skyrings/skyring-common/event"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultUserName = "admin"
	DefaultPassword = "admin"
	DefaultEmail    = "admin@localhost"
	DefaultRole     = "admin"
)

func logUserEvent(eventtype string, message string, description string, ctxt string, severity models.AlarmStatus) error {
	var EventType = map[string]string{
		"USER_LOGGED_IN":  "User logged in",
		"USER_LOGGED_OUT": "User logged out",
		"USER_ADDED":      "User added",
		"USER_MODIFIED":   "User modified",
		"USER_DELETED":    "User deleted",
	}

	var event models.AppEvent
	eventId, err := uuid.New()
	if err != nil {
		logger.Get().Error("%s-Uuid generation for the event failed for event: %s. error: %v", ctxt, event.Name, err)
		return err
	}
	event.EventId = *eventId
	event.NotificationEntity = models.NOTIFICATION_ENTITY_USER
	event.Timestamp = time.Now()
	event.Notify = false
	event.Name = EventType[eventtype]
	event.Message = message
	event.Description = description

	event.Severity = severity
	if err := common_event.AuditLog(ctxt, event, GetDbProvider()); err != nil {
		logger.Get().Error("%s- Error logging the event: %s. Error:%v", ctxt, event.Name, err)
		return err
	}
	return nil
}

func AddDefaultUser() error {

	defaultUser := models.User{Username: DefaultUserName, Email: DefaultEmail, Role: DefaultRole}

	if err := GetAuthProvider().AddUser(defaultUser, DefaultPassword); err != nil {
		logger.Get().Error("Unable to create default User:%s", err)
		return err
	}

	return nil
}

func (a *App) login(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	type apiError APIError
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("Error parsing http request body:%s", err)
		HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		HandleHttpError(rw, err)
		return
	}
	if err := GetAuthProvider().Login(rw, req, m["username"].(string), m["password"].(string)); err != nil {
		logger.Get().Error("Unable to login User:%s", err)
		bytes, _ := json.Marshal(apiError{Error: err.Error()})
		message := fmt.Sprintf("Log in failed for user: %s", m["username"])
		description := fmt.Sprintf("Log in failed for user: %s .Error: %s", m["username"], err)
		if err := logUserEvent("USER_LOGGED_IN", message, description, ctxt, models.ALARM_STATUS_MINOR); err != nil {
			logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
		}
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write(bytes)
		return
	}
	message := fmt.Sprintf("User: %s logged in", m["username"])
	if err := logUserEvent("USER_LOGGED_IN", message, message, ctxt, models.ALARM_STATUS_CLEARED); err != nil {
		logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
	}

	bytes := []byte(`{"message": "Logged in"}`)
	rw.Write(bytes)
}

func (a *App) logout(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}
	user := strings.Split(ctxt, ":")[0]
	if err := GetAuthProvider().Logout(rw, req); err != nil {
		logger.Get().Error("Unable to logout User:%s", err)
		message := fmt.Sprintf("Log out failed for user: %s", user)
		description := fmt.Sprintf("Log out failed for user: %s Error: %v", user, err)
		if err := logUserEvent("USER_LOGGED_OUT", message, description, ctxt, models.ALARM_STATUS_MINOR); err != nil {
			logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
		}
		HandleHttpError(rw, err)
		return
	}
	message := fmt.Sprintf("User: %s logged out", user)
	if err := logUserEvent("USER_LOGGED_OUT", message, message, ctxt, models.ALARM_STATUS_MINOR); err != nil {
		logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
	}

	bytes := []byte(`{"message": "Logged out"}`)
	rw.Write(bytes)
}

func (a *App) getUsers(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("%s-Error Getting the context. error: %v", ctxt, err)
	}

	users, err := GetAuthProvider().ListUsers()
	if err != nil {
		logger.Get().Error("%s-Unable to List the users:%s", ctxt, err)
		HandleHttpError(rw, err)
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
		logger.Get().Error("%s-Unable to marshal the list of Users:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}

func (a *App) getUser(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(req)

	user, err := GetAuthProvider().GetUser(vars["username"], req)
	if err != nil {
		logger.Get().Error("%s-Unable to Get the user:%s", ctxt, err)
		HandleHttpError(rw, err)
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
		logger.Get().Error("%s-Unable to marshal the User:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}

func (a *App) getExternalUsers(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	pageNo := 1 //Default page number or Home page number
	pageNumberStr := req.URL.Query().Get("pageno")
	if pageNumberStr != "" {
		pageNo, err = strconv.Atoi(pageNumberStr)
		// Check for invalid input or conversion error
		if err != nil {
			logger.Get().Error("%s-Error getting pageno:%s", ctxt, err)
			HandleHttpError(rw, err)
			return
		}
		// Pressing back button from ui on page 1 may request for
		// page 0. Set page number to home page / first page number
		// (which is number 1) if the request page no is negative
		if pageNo < 1 {
			pageNo = 1
		}
	}

	pageSize := models.LDAP_USERS_PER_PAGE //Default page size
	pageSizeStr := req.URL.Query().Get("pagesize")
	if pageSizeStr != "" {
		pageSize, err = strconv.Atoi(pageSizeStr)
		// Check for invalid input or conversion error
		if err != nil {
			logger.Get().Error("%s-Error getting pagesize:%s", ctxt, err)
			HandleHttpError(rw, err)
			return
		}
		if pageSize > models.LDAP_USERS_PER_PAGE || pageSize < 1 {
			pageSize = models.LDAP_USERS_PER_PAGE
		}
	}

	search := req.URL.Query().Get("search")
	externalUsers, err := GetAuthProvider().ListExternalUsers(search, pageNo, pageSize)
	if err != nil {
		logger.Get().Error("%s-Unable to List the users:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	//hide the hash field for users in the list
	type PublicUser struct {
		*models.User
		Hash         bool `json:"hash,omitempty"`
		Notification bool `json:"notification,omitempty"`
	}

	var pUsers []PublicUser
	for _, user := range externalUsers.Users {

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

	json.NewEncoder(rw).Encode(
		struct {
			TotalCount int          `json:"totalcount"`
			StartIndex int          `json:"startindex"`
			EndIndex   int          `json:"endindex"`
			Users      []PublicUser `json:"users"`
		}{externalUsers.TotalCount, externalUsers.StartIndex, externalUsers.EndIndex, pUsers})
}

func (a *App) addUsers(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	var user models.User

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("%s-Unable to Unmarshall the data:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	var password string
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
	if val, ok := m["password"]; ok {
		password = val.(string)
	}

	if err := GetAuthProvider().AddUser(user, password); err != nil {
		message := fmt.Sprintf("Addition of user: %s failed", user.Username)
		description := fmt.Sprintf("Addition of user: %s failed. Reason: %v", user.Username, err)
		if err := logUserEvent("USER_ADDED", message, description, ctxt, models.ALARM_STATUS_MINOR); err != nil {
			logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
		}
		logger.Get().Error("%s-Unable to create User:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	message := fmt.Sprintf("New user: %s added to skyring", user.Username)
	description := fmt.Sprintf("New user: %s with role: %s added to skyring", user.Username, user.Role)
	if err := logUserEvent("USER_ADDED", message, description, ctxt, models.ALARM_STATUS_CLEARED); err != nil {
		logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
	}

}

func (a *App) modifyUsers(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(req)
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("%s-Unable to Unmarshall the data:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}

	if err := GetAuthProvider().UpdateUser(vars["username"], m); err != nil {
		message := fmt.Sprintf("User settings modification failed for user: %s", vars["username"])
		description := fmt.Sprintf("User settings modification failed for user: %s. Error: %v", vars["username"], err)
		if err := logUserEvent("USER_MODIFIED", message, description, ctxt, models.ALARM_STATUS_MINOR); err != nil {
			logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
		}

		logger.Get().Error("%s-Unable to update user:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	message := fmt.Sprintf("User settings has been modified for user: %s", vars["username"])
	description := fmt.Sprintf("User settings has been modified for user: %s. Changed attributes:", vars["username"])

	if val, ok := m["email"]; ok {
		description += fmt.Sprintf(" Email: %s", val.(string))
	}
	if val, ok := m["notificationenabled"]; ok {
		description += fmt.Sprintf(" Notification Enabled: %v", val.(bool))
	}
	if val, ok := m["status"]; ok {
		description += fmt.Sprintf(" Enabled: %v", val.(bool))
	}

	if err := logUserEvent("USER_MODIFIED", message, description, ctxt, models.ALARM_STATUS_CLEARED); err != nil {
		logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
	}

}

func (a *App) deleteUser(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	vars := mux.Vars(req)

	if err := GetAuthProvider().DeleteUser(vars["username"]); err != nil {
		message := fmt.Sprintf("User deletion failed for user: %s", vars["username"])
		description := fmt.Sprintf("User deletion failed for user: %s. Error: %v", vars["username"], err)
		if err := logUserEvent("USER_DLELTED", message, description, ctxt, models.ALARM_STATUS_MINOR); err != nil {
			logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
		}
		logger.Get().Error("%s-Unable to delete User:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	message := fmt.Sprintf("User :%s has been deleted ", vars["username"])
	if err := logUserEvent("USER_DLELTED", message, message, ctxt, models.ALARM_STATUS_CLEARED); err != nil {
		logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
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

func (a *App) getLdapConfig(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}
	ldapConfig, err := GetAuthProvider().GetDirectory()
	if err != nil {
		logger.Get().Error("%s-Unable to reterive directory service configuration:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}

	json.NewEncoder(rw).Encode(
		struct {
			LdapServer  string `json:"ldapserver"`
			Port        uint   `json:"port"`
			Base        string `json:"base"`
			DomainAdmin string `json:"domainadmin"`
			Password    string `json:"password"`
			Uid         string `json:"uid"`
			FirstName   string `json:"firstname"`
			LastName    string `json:"lastname"`
			DisplayName string `json:"displayname"`
			Email       string `json:"email"`
		}{ldapConfig.LdapServer, ldapConfig.Port, ldapConfig.Base, ldapConfig.DomainAdmin,
			"", ldapConfig.Uid, ldapConfig.FirstName, ldapConfig.LastName,
			ldapConfig.DisplayName, ldapConfig.Email})
}

func (a *App) configLdap(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	var directory models.Directory

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("%s-Unable to Unmarshall the data:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}

	//var password string
	if val, ok := m["ldapserver"]; ok {
		directory.LdapServer = val.(string)
	}
	if val, ok := m["port"]; ok {
		directory.Port = uint(val.(float64))
	}
	logger.Get().Info("Port :%d", directory.Port)
	if val, ok := m["base"]; ok {
		directory.Base = val.(string)
	}
	if val, ok := m["domainadmin"]; ok {
		directory.DomainAdmin = val.(string)
	}
	if val, ok := m["password"]; ok {
		directory.Password = val.(string)
	}
	if val, ok := m["uid"]; ok {
		directory.Uid = val.(string)
	}
	if val, ok := m["firstname"]; ok {
		directory.FirstName = val.(string)
	}
	if val, ok := m["lastname"]; ok {
		directory.LastName = val.(string)
	}
	if val, ok := m["displayname"]; ok {
		directory.DisplayName = val.(string)
	}
	if val, ok := m["email"]; ok {
		directory.Email = val.(string)
	}
	user := strings.Split(ctxt, ":")[0]

	if err := GetAuthProvider().SetDirectory(directory); err != nil {
		message := fmt.Sprintf("Configuring LDAP failed, Tried by user: %s", user)
		description := fmt.Sprintf("Configuring LDAP failed, Tried by user: %s. Error: %s", user, err)
		if err := logUserEvent("USER_DLELTED", message, description, ctxt, models.ALARM_STATUS_MINOR); err != nil {
			logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
		}
		logger.Get().Error("%s-Unable to configure directory service:%s", ctxt, err)
		HandleHttpError(rw, err)
		return
	}
	message := fmt.Sprintf("LDAP configuration updated successfully by user: %s", user)
	if err := logUserEvent("USER_DLELTED", message, message, ctxt, models.ALARM_STATUS_CLEARED); err != nil {
		logger.Get().Error("%s- Unable to log User event. Error: %v", ctxt, err)
	}
}
