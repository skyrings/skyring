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
	"github.com/goincremental/negroni-sessions"
	"github.com/golang/glog"
	"github.com/skyrings/skyring/authprovider"
	"github.com/skyrings/skyring/models"
	"golang.org/x/crypto/bcrypt"
	"io"
	"io/ioutil"
	"net/http"
)

const ProviderName = "localauthprovider"

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
	backend     authprovider.AuthBackend
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
		glog.Errorln("missing configuration file for Local Auth provider")
		return nil, fmt.Errorf("missing configuration file for Local Auth provider")
	}

	providerCfg := LocalProviderCfg{}

	bytes, err := ioutil.ReadAll(config)
	if err != nil {
		glog.Errorf("Error reading Configuration file:%s", err)
		return nil, err
	}
	if err = json.Unmarshal(bytes, &providerCfg); err != nil {
		glog.Errorf("Unable to Unmarshall the data:%s", err)
		return nil, err
	}
	//Create DB Backend
	backend, err := authprovider.NewMongodbBackend()
	if err != nil {
		glog.Errorf("Unable to initialize the DB backend for localauthprovider:%s", err)
		panic(err)
	}
	//Create the Provider
	if provider, err := NewAuthorizer(backend, providerCfg.DefaultRole, providerCfg.Roles); err != nil {
		glog.Errorf("Unable to initialize the authorizer for localauthprovider:%s", err)
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

func NewAuthorizer(backend authprovider.AuthBackend, defaultRole string, roles map[string]Role) (Authorizer, error) {
	var a Authorizer
	a.backend = backend
	a.roles = roles
	a.defaultRole = defaultRole
	if _, ok := roles[defaultRole]; !ok {
		glog.Errorln("Default role provided is not valid")
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
	session := sessions.GetSession(req)
	if sess := session.Get("username"); sess != nil {
		glog.Errorln("Already logged in")
		return nil
	}

	if user, err := a.backend.User(u); err == nil {
		if user.Status {
			verify := bcrypt.CompareHashAndPassword(user.Hash, []byte(p))
			if verify != nil {
				glog.Errorln("Passwords Doesnot match")
				return mkerror("password doesn't match")
			}
		} else {
			glog.Errorln("This user is not allowed. Status Disabled")
			return mkerror("This user is not allowed. Status Disabled")
		}
	} else {
		glog.Errorln("User Not Found")
		return mkerror("user not found")
	}
	session.Set("username", u)

	return nil
}

// Register and save a new user. Returns an error and adds a message if the
// username is in use.
//
// Pass in a instance of UserData with at least a username and email specified. If no role
// is given, the default one is used.

func (a Authorizer) AddUser(user models.User, password string) error {
	if user.Username == "" {
		glog.Errorln("no user name given")
		return mkerror("no username given")
	}
	if user.Email == "" {
		glog.Errorln("no email given")
		return mkerror("no email given")
	}
	if password == "" {
		glog.Errorln("no password given")
		return mkerror("no password given")
	}

	//Set the usertype to internal
	user.Type = authprovider.Internal
	user.Status = true

	// Validate username
	_, err := a.backend.User(user.Username)
	if err == nil {
		glog.Errorln("Username already exists")
		return mkerror("user already exists")
	} else if err.Error() != ErrMissingUser.Error() {
		if err != nil {
			glog.Errorf("Error retrieving user:%s", err)
			return mkerror(err.Error())
		}
		return nil
	}

	// Generate and save hash
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		glog.Errorf("couldn't save password:%s", err)
		return mkerror("couldn't save password: " + err.Error())
	}
	user.Hash = hash

	// Validate role
	if user.Role == "" {
		user.Role = a.defaultRole
	} else {
		if _, ok := a.roles[user.Role]; !ok {
			glog.Errorln("Non Existing Role")
			return mkerror("non-existant role")
		}
	}

	err = a.backend.SaveUser(user)
	if err != nil {
		glog.Errorf("Erro Saving the User:%s", err)
		return mkerror(err.Error())
	}
	return nil
}

// Update changes data for an existing user. Needs thought...
//Just added for completeness. Will revisit later
func (a Authorizer) UpdateUser(req *http.Request, username string, p string, e string) error {
	var (
		hash  []byte
		email string
	)

	user, err := a.backend.User(username)
	if err != nil {
		glog.Errorln("Error retrieving the user:%s", err)
		return err
	}
	if p != "" {
		hash, err = bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
		if err != nil {
			glog.Errorln("Error saving the password:%s", err)
			return mkerror("couldn't save password: " + err.Error())
		}
	} else {
		hash = user.Hash
	}
	if e != "" {
		email = e
	} else {
		email = user.Email
	}

	newuser := models.User{Username: username, Email: email, Hash: hash, Role: user.Role}

	err = a.backend.SaveUser(newuser)
	if err != nil {
		glog.Errorln("Error saving the user to DB:%s", err)
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
	session := sessions.GetSession(req)
	session.Delete("username")
	return nil
}

// CurrentUser returns the currently logged in user and a boolean validating
// the information.
func (a Authorizer) GetUser(u string) (user models.User, e error) {

	user, e = a.backend.User(u)
	if e != nil {
		glog.Errorf("Error retrieving the user:%s", e)
		return user, e
	}
	return user, nil
}

func (a Authorizer) ListUsers() (users []models.User, err error) {

	if users, err = a.backend.Users(); err != nil {
		glog.Errorf("Unable get the list of Users: %v", err)
		return users, err
	}
	return users, nil
}

func (a Authorizer) ListExternalUsers() (users []models.User, err error) {
	return users, errors.New("Not Supported")
}

// DeleteUser removes a user from the Authorize. ErrMissingUser is returned if
// the user to be deleted isn't found.
func (a Authorizer) DeleteUser(username string) error {
	err := a.backend.DeleteUser(username)
	if err != nil {
		glog.Errorf("Unable delete the user: %v", err)
		return err
	}
	return nil
}
