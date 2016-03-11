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
package localauthprovider

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/skyrings/skyring-common/dao"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/authprovider"
	"golang.org/x/crypto/bcrypt"
	"io"
	"io/ioutil"
	"net/http"
)

const (
	ProviderName = "localauthprovider"
)

// ErrDeleteNull is returned by DeleteUser when that user didn't exist at the
// time of call.
// ErrMissingUser is returned by Users when a user is not found.
var (
	ErrDeleteNull  = mkerror("deleting non-existant user")
	ErrMissingUser = mkerror("can't find user")
)

// Role represents an interal role. Roles are essentially a string mapped to an
// integer. Roles must be greater than zero.
type Role int

// Authorizer structures contain the store of user session cookies a reference
// to a backend storage system.
type Authorizer struct {
	userDao     dao.UserInterface
	defaultRole string
	roles       map[string]Role
}

type LocalProviderCfg struct {
	Roles       map[string]Role
	DefaultRole string
}

func init() {
	authprovider.RegisterAuthProvider(ProviderName, func(config io.Reader) (authprovider.AuthInterface, error) {
		return NewLocalAuthProvider(config)
	})
}

func mkerror(msg string) error {
	return errors.New(msg)
}

func NewLocalAuthProvider(config io.Reader) (*Authorizer, error) {
	if config == nil {
		logger.Get().Error("missing configuration file for Local Auth provider")
		return nil, fmt.Errorf("missing configuration file for Local Auth provider")
	}

	providerCfg := LocalProviderCfg{}

	bytes, err := ioutil.ReadAll(config)
	if err != nil {
		logger.Get().Error("Error reading configuration file. error: %v", err)
		return nil, err
	}
	if err = json.Unmarshal(bytes, &providerCfg); err != nil {
		logger.Get().Error("Unable to Unmarshall the data. error: %v", err)
		return nil, err
	}
	//Create DB Backend
	userDao := skyring.GetDbProvider().UserInterface()

	//Create the Provider
	if provider, err := NewAuthorizer(userDao, providerCfg.DefaultRole, providerCfg.Roles); err != nil {
		logger.Get().Error("Unable to initialize the authorizer for localauthprovider. error: %v", err)
		panic(err)
	} else {
		return &provider, nil
	}
}

// NewAuthorizer returns a new Authorizer given an AuthBackend
// Roles are a map of string to httpauth.Role values (integers). Higher Role values
// have more access.
//
// Example roles:
//
//     var roles map[string]httpauth.Role
//     roles["user"] = 2
//     roles["admin"] = 4
//     roles["moderator"] = 3

func NewAuthorizer(userDao dao.UserInterface, defaultRole string, roles map[string]Role) (Authorizer, error) {
	var a Authorizer
	a.userDao = userDao
	a.roles = roles
	a.defaultRole = defaultRole
	if _, ok := roles[defaultRole]; !ok {
		logger.Get().Error("Default role provided is not valid")
		return a, mkerror("defaultRole missing")
	}
	return a, nil
}

// ProviderName returns the auth provider ID.
func (a Authorizer) ProviderName() string {
	return ProviderName
}

// Login logs a user in. They will be redirected to dest or to the last
// location an authorization redirect was triggered (if found) on success. A
// message will be added to the session on failure with the reason.

func (a Authorizer) Login(rw http.ResponseWriter, req *http.Request, u string, p string) error {
	session, err := skyring.Store.Get(req, "session-key")
	if err != nil {
		logger.Get().Error("Error getting the session for user: %s. error: %v", u, err)
		return err
	}
	if !session.IsNew {
		if session.Values["username"] == u {
			logger.Get().Info("User: %s already logged in", u)
			return nil
		} else {
			return mkerror("user " + session.Values["username"].(string) + " is already logged in")
		}
	}
	if user, err := a.userDao.User(u); err == nil {
		if user.Type == authprovider.Internal && user.Status {
			verify := bcrypt.CompareHashAndPassword(user.Hash, []byte(p))
			if verify != nil {
				logger.Get().Error("Password does not match for user: %s", u)
				return mkerror("password doesn't match")
			}
		} else {
			logger.Get().Error("User: %s is not allowed by localauthprovider", u)
			return mkerror("This user is not allowed by localauthprovider")
		}
	} else {
		logger.Get().Error("User: %s not found", u)
		return mkerror("user not found")
	}

	// Update the new username in session before persisting to DB
	session.Values["username"] = u
	if err = session.Save(req, rw); err != nil {
		logger.Get().Error("Error saving the session for user: %s. error: %v", u, err)
		return err
	}

	return nil
}

// Register and save a new user. Returns an error and adds a message if the
// username is in use.
//
// Pass in a instance of UserData with at least a username and email specified. If no role
// is given, the default one is used.

func (a Authorizer) AddUser(user models.User, password string) error {
	if user.Username == "" {
		logger.Get().Error("no user name given")
		return mkerror("no username given")
	}
	if user.Email == "" {
		logger.Get().Error("no email given")
		return mkerror("no email given")
	}
	if password == "" {
		logger.Get().Error("no password given")
		return mkerror("no password given")
	}

	//Set the usertype to internal
	user.Type = authprovider.Internal
	user.Status = true

	// Validate username
	_, err := a.userDao.User(user.Username)
	if err == nil {
		logger.Get().Error("Username: %s already exists", user.Username)
		return mkerror("user already exists")
	} else if err.Error() != ErrMissingUser.Error() {
		if err != nil {
			logger.Get().Error("Error retrieving user: %s. error: %v", user.Username, err)
			return mkerror(err.Error())
		}
		return nil
	}

	// Generate and save hash
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		logger.Get().Error("couldn't save password for user: %s. error: %v", user.Username, err)
		return mkerror("couldn't save password: " + err.Error())
	}
	user.Hash = hash

	// Validate role
	if user.Role == "" {
		user.Role = a.defaultRole
	} else {
		if _, ok := a.roles[user.Role]; !ok {
			logger.Get().Error("Non Existing Role: %s", user.Role)
			return mkerror("non-existant role")
		}
	}

	err = a.userDao.SaveUser(user)
	if err != nil {
		logger.Get().Error("Erro Saving the User: %s. error: %v", user, err)
		return mkerror(err.Error())
	}
	return nil
}

// Update changes data for an existing user. Needs thought...
//Just added for completeness. Will revisit later
func (a Authorizer) UpdateUser(username string, m map[string]interface{}, req *http.Request) error {
	var (
		hash []byte
	)

	user, err := a.userDao.User(username)
	if err != nil {
		logger.Get().Error("Error retrieving the user: %s. error: %v", username, err)
		return err
	}

	if val, ok := m["oldpassword"]; ok {
		op := val.(string)
		match := bcrypt.CompareHashAndPassword(user.Hash, []byte(op))
		if match != nil {
			logger.Get().Error("Old password doesnt match")
			return mkerror("Old password doesnt match" + err.Error())
		} else {
			if val, ok := m["password"]; ok {
				p := val.(string)
				hash, err = bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
				if err != nil {
					logger.Get().Error("Error saving the password for user: %s. error: %v", username, err)
					return mkerror("couldn't save password: " + err.Error())
				}
				user.Hash = hash
			}
		}
	} else {
		var u string
		session, err := skyring.Store.Get(req, "session-key")
		if err != nil {
			logger.Get().Error("Error getting the session. error: %v", err)
			return err
		}
		if val, ok := session.Values["username"]; ok {
			u = val.(string)
		} else {
			logger.Get().Error("Unable to identify the user from session")
			return mkerror("Unable to identify the user from session")
		}
		curUser, e := a.userDao.User(u)
		if e != nil {
			logger.Get().Error("Error retrieving the user: %s. error: %v", u, e)
			return e
		}
		if curUser.Role == "admin" {
			if val, ok := m["password"]; ok {
				p := val.(string)
				hash, err = bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
				if err != nil {
					logger.Get().Error("Error saving the password for user: %s. error: %v", username, err)
					return mkerror("couldn't save password: " + err.Error())
				}
				user.Hash = hash
			}
		} else {
			logger.Get().Error("Error saving the password for user since no previledge: %s. error: %v", username, err)
			return mkerror("couldn't save password: " + err.Error())
		}
	}

	if val, ok := m["email"]; ok {
		e := val.(string)
		user.Email = e
	}

	if val, ok := m["notificationenabled"]; ok {
		n := val.(bool)
		user.NotificationEnabled = n
	}

	if val, ok := m["status"]; ok {
		s := val.(bool)
		user.Status = s
	}

	err = a.userDao.SaveUser(user)
	if err != nil {
		logger.Get().Error("Error saving the user: %s to DB. error: %v", username, err)
		return err
	}

	return nil
}

// Authorize checks if a user is logged in and returns an error on failed
// authentication. If redirectWithMessage is set, the page being authorized
// will be saved and a "Login to do that." message will be saved to the
// messages list. The next time the user logs in, they will be redirected back
// to the saved page.
func (a Authorizer) Authorize(rw http.ResponseWriter, req *http.Request) error {
	return nil
}

// AuthorizeRole runs Authorize on a user, then makes sure their role is at
// least as high as the specified one, failing if not.
func (a Authorizer) AuthorizeRole(rw http.ResponseWriter, req *http.Request, role string) error {
	return nil
}

// Logout clears an authentication session and add a logged out message.
func (a Authorizer) Logout(rw http.ResponseWriter, req *http.Request) error {
	session, err := skyring.Store.Get(req, "session-key")
	if err != nil {
		logger.Get().Error("Error getting the session. error: %v", err)
		return err
	}
	session.Options.MaxAge = -1
	if err = session.Save(req, rw); err != nil {
		logger.Get().Error("Error saving the session. error: %v", err)
		return err
	}
	return nil
}

// CurrentUser returns the currently logged in user and a boolean validating
// the information.
func (a Authorizer) GetUser(u string, req *http.Request) (user models.User, e error) {
	//if username is me, get the currently loggedin user
	if u == authprovider.CurrentUser {
		session, err := skyring.Store.Get(req, "session-key")
		if err != nil {
			logger.Get().Error("Error getting the session. error: %v", err)
			return user, err
		}
		if val, ok := session.Values["username"]; ok {
			u = val.(string)
		} else {
			logger.Get().Error("Unable to identify the user from session")
			return user, mkerror("Unable to identify the user from session")
		}
	}
	user, e = a.userDao.User(u)
	if e != nil {
		logger.Get().Error("Error retrieving the user: %s. error: %v", u, e)
		return user, e
	}
	return user, nil
}

func (a Authorizer) ListUsers() (users []models.User, err error) {
	if users, err = a.userDao.Users(nil); err != nil {
		logger.Get().Error("Unable get the list of users. error: %v", err)
		return users, err
	}
	return users, nil
}

func (a Authorizer) ListExternalUsers(search string, page, count int) (externalUsers models.ExternalUsers, err error) {
	return externalUsers, errors.New("Not Supported")
}

// DeleteUser removes a user from the Authorize. ErrMissingUser is returned if
// the user to be deleted isn't found.
func (a Authorizer) DeleteUser(username string) error {
	err := a.userDao.DeleteUser(username)
	if err != nil {
		logger.Get().Error("Unable delete the user: %s. error: %v", username, err)
		return err
	}
	return nil
}

func (a Authorizer) GetDirectory() (directory models.Directory, err error) {
	return directory, errors.New("Not Supported")
}

func (a Authorizer) SetDirectory(directory models.Directory) error {
	return errors.New("Not Supported")
}
