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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	skyringmodel "github.com/skyrings/skyring/models"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
)

const (
	CipherKey       = "Skyring - RedHat"
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
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Write(bytes)
		return
	}
	bytes := []byte(`{"message": "Logged in"}`)
	rw.Write(bytes)
}

func (a *App) logout(rw http.ResponseWriter, req *http.Request) {
	if err := GetAuthProvider().Logout(rw, req); err != nil {
		logger.Get().Error("Unable to logout User:%s", err)
		HandleHttpError(rw, err)
		return
	}
	bytes := []byte(`{"message": "Logged out"}`)
	rw.Write(bytes)
}

func (a *App) getUsers(rw http.ResponseWriter, req *http.Request) {

	users, err := GetAuthProvider().ListUsers()
	if err != nil {
		logger.Get().Error("Unable to List the users:%s", err)
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
		logger.Get().Error("Unable to marshal the list of Users:%s", err)
		HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}

func (a *App) getUser(rw http.ResponseWriter, req *http.Request) {

	vars := mux.Vars(req)

	user, err := GetAuthProvider().GetUser(vars["username"], req)
	if err != nil {
		logger.Get().Error("Unable to Get the user:%s", err)
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
		logger.Get().Error("Unable to marshal the User:%s", err)
		HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}

func (a *App) getExternalUsers(rw http.ResponseWriter, req *http.Request) {

	var err error
	pageNo := 1 //Default page number or Home page number
	pageNumberStr := req.URL.Query().Get("pageno")
	if pageNumberStr != "" {
		pageNo, err = strconv.Atoi(pageNumberStr)
		// Check for invalid input or conversion error
		if err != nil {
			logger.Get().Error("Error getting pageno:%s", err)
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
			logger.Get().Error("Error getting pagesize:%s", err)
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
		logger.Get().Error("Unable to List the users:%s", err)
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

	var user models.User

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
		logger.Get().Error("Unable to create User:%s", err)
		HandleHttpError(rw, err)
		return
	}
}

func (a *App) modifyUsers(rw http.ResponseWriter, req *http.Request) {

	vars := mux.Vars(req)
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

	if err := GetAuthProvider().UpdateUser(vars["username"], m); err != nil {
		logger.Get().Error("Unable to update user:%s", err)
		HandleHttpError(rw, err)
		return
	}

}

func (a *App) deleteUser(rw http.ResponseWriter, req *http.Request) {

	vars := mux.Vars(req)

	if err := GetAuthProvider().DeleteUser(vars["username"]); err != nil {
		logger.Get().Error("Unable to delete User:%s", err)
		HandleHttpError(rw, err)
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

func (a *App) getLdapConfig(rw http.ResponseWriter, req *http.Request) {
	ldapConfig, err := GetDirectory()
	if err != nil {
		logger.Get().Error("Unable to reterive directory service configuration:%s", err)
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

func GetAuthProviderInfo(providerName string) (skyringmodel.AuthConfig, error) {
	session := db.GetDatastore()
	authConf := skyringmodel.AuthConfig{}
	c := session.DB(conf.SystemConfig.DBConfig.Database).C(AuthProvider)
	err := c.Find(bson.M{"providername": providerName}).One(&authConf)
	return authConf, err
}

func (a *App) getDbAuthProvider(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	authConf, err := GetAuthProviderInfo(vars["provider"])
	if err != nil || len(authConf.ProviderName) == 0 {
		logger.Get().Error("Failed to reterive provider details: %s", err)
		HandleHttpError(rw, err)
		return
	}
	bytes, err := json.Marshal(authConf)
	if err != nil {
		logger.Get().Error("Unable to marshal the provider config details: %s", err)
		HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)
}

func (a *App) setDbAuthProvider(rw http.ResponseWriter, req *http.Request) {
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

	var providerName, providerConf string
	var providerState bool
	if val, ok := m["providername"]; ok {
		providerName = val.(string)
	} else {
		logger.Get().Error("ProviderName not provided :%s", err)
		HandleHttpError(rw, err)
		return
	}
	if val, ok := m["status"]; ok {
		providerState = val.(bool)
	} else {
		logger.Get().Error("ProviderState not provided :%s", err)
		HandleHttpError(rw, err)
		return
	}
	if val, ok := m["confpath"]; ok {
		providerConf = val.(string)
	}
	// Check ldap db available if the enabling provider name is ldap
	if providerState && providerName == LdapAuthProvider {
		_, err := GetDirectory()
		if err != nil {
			logger.Get().Error("Unable to find directory service details:%s", err)
			HandleHttpError(rw, err)
			return
		}
	}
	// Update provider state into authconfig db
	err = UpdateProvider(providerName, providerState, providerConf)
	if err != nil {
		logger.Get().Error("Failed to set provider state :%s", err)
		HandleHttpError(rw, err)
		return
	}
	err = SetAuthProvider()
	if err != nil {
		logger.Get().Error("failed to set auth provider :%v", err)
		HandleHttpError(rw, err)
		return
	}
}

func (a *App) configLdap(rw http.ResponseWriter, req *http.Request) {

	var directory models.Directory

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

	if err := SetDirectory(directory); err != nil {
		logger.Get().Error("Unable to configure directory service:%s", err)
		HandleHttpError(rw, err)
		return
	}
}

func GetDirectory() (directory models.Directory, err error) {
	session := db.GetDatastore()
	c := session.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_LDAP)
	val, err := c.Count()
	if val > 0 {
		err = c.Find(bson.M{}).One(&directory)
		if err != nil {
			logger.Get().Error("Failed to get ldap config details %s", err)
			return directory, err
		}
	}
	return directory, nil
}

func UpdateProvider(providerName string, state bool, providerPath string) (err error) {
	// Check whether the provider exists
	authConf, _ := GetAuthProviderInfo(providerName)
	session := db.GetDatastore()
	authConfDb := session.DB(conf.SystemConfig.DBConfig.Database).C(AuthProvider)

	// Recreate the provider if it does not exists
	if len(authConf.ProviderName) == 0 {
		authConfDb.Insert(skyringmodel.AuthConfig{providerName, providerPath, state})
	} else {
		// Update the provider if it's already exists
		search := bson.M{"providername": providerName}
		change := bson.M{"providername": providerName, "confpath": providerPath, "status": state}
		err = authConfDb.Update(search, change)
		if err != nil {
			logger.Get().Error("failed to update auth provider :%v", err)
			return err
		}
	}

	// Enable or disable other provider state based on this provider state
	// We can disable all the remaining provider if the current provider state is enable
	// otherwise only one will be enabled.
	search := bson.M{"status": state, "providername": bson.M{"$ne": providerName}}
	change := bson.M{"$set": bson.M{"status": !state}}
	if !state {
		err = authConfDb.Update(search, change)
	} else {
		_, err = authConfDb.UpdateAll(search, change)
	}
	if err != nil {
		logger.Get().Error("failed to set auth provider state:%v", err)
		return err
	}
	return nil
}

func SetDirectory(directory models.Directory) error {
	if directory.LdapServer == "" {
		logger.Get().Error("no directory server name provided")
		return errors.New("no directory server name provided")
	}
	if directory.Port == 0 {
		logger.Get().Error("no directory server port number provided")
		return errors.New("no directory server port number provided")
	}
	if directory.Base == "" {
		logger.Get().Error("no directory server connection string provided")
		return errors.New("no directory server connection string provided")
	}

	session := db.GetDatastore()
	c := session.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_LDAP)
	val, err := c.Count()
	if err != nil {
		logger.Get().Error("failed to get ldap records count")
		return errors.New("failed to get ldap records count")
	}
	if val > 0 {
		c.RemoveAll(nil)
	}

	pswd := []byte(directory.Password)
	block, err := aes.NewCipher([]byte(CipherKey))
	if err != nil {
		logger.Get().Info("Failed to generate cipher:%s", err)
		return errors.New("Failed to generate cipher")
	}
	chkey := make([]byte, aes.BlockSize+len(pswd))
	iv := chkey[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		logger.Get().Info("Failed to configure ldap:%s", err)
		return errors.New("Failed to configure ldap")
	}
	stream := cipher.NewOFB(block, iv)
	stream.XORKeyStream(chkey[aes.BlockSize:], pswd)

	err = c.Insert(&models.Directory{directory.LdapServer, directory.Port, directory.Base,
		directory.DomainAdmin, string(chkey), directory.Uid,
		directory.FirstName, directory.LastName, directory.DisplayName, directory.Email})

	if err != nil {
		logger.Get().Error("Failed to update  :%v", err)
		return err
	}
	return nil
}
