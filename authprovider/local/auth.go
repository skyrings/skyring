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
	"github.com/gorilla/sessions"
	"github.com/skyrings/skyring/authprovider"
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
	cookiejar   *sessions.CookieStore
	backend     AuthBackend
	defaultRole string
	roles       map[string]Role
}

// The AuthBackend interface defines a set of methods an AuthBackend must
// implement.
type AuthBackend interface {
	SaveUser(u authprovider.User) error
	User(username string) (user authprovider.User, e error)
	Users() (users []authprovider.User, e error)
	DeleteUser(username string) error
	Close()
}

type LocalProviderCfg struct {
	MongoURL    string
	Database    string
	Roles       map[string]Role
	DefaultRole string
}

func init() {
	authprovider.RegisterAuthProvider(ProviderName, func(config io.Reader) (authprovider.AuthInterface, error) {
		return NewLocalAuthProvider(config)
	})
}

func mkerror(msg string) error {
	return errors.New("localprovider: " + msg)
}

func NewLocalAuthProvider(config io.Reader) (*Authorizer, error) {
	if config == nil {
		return nil, fmt.Errorf("missing configuration file for Local Auth provider")
	}

	providerCfg := LocalProviderCfg{}

	if bytes, err := ioutil.ReadAll(config); err != nil {
		return nil, err
	} else {
		if err = json.Unmarshal(bytes, &providerCfg); err != nil {
			return nil, err
		}
	}

	//Create DB Backend
	backend, err := NewMongodbBackend(providerCfg.MongoURL, providerCfg.Database)
	if err != nil {
		panic(err)
	}

	//Create the Provider
	if provider, err := NewAuthorizer(backend, []byte("cookie-encryption-key"), providerCfg.DefaultRole, providerCfg.Roles); err != nil {
		return nil, err
	} else {
		return &provider, nil
	}

}

// NewAuthorizer returns a new Authorizer given an AuthBackend, a cookie store
// key, a default user role, and a map of roles. If the key changes, logged in
// users will need to reauthenticate.
//
// Roles are a map of string to httpauth.Role values (integers). Higher Role values
// have more access.
//
// Example roles:
//
//     var roles map[string]httpauth.Role
//     roles["user"] = 2
//     roles["admin"] = 4
//     roles["moderator"] = 3

func NewAuthorizer(backend AuthBackend, key []byte, defaultRole string, roles map[string]Role) (Authorizer, error) {
	var a Authorizer
	a.cookiejar = sessions.NewCookieStore([]byte(key))
	a.backend = backend
	a.roles = roles
	a.defaultRole = defaultRole
	if _, ok := roles[defaultRole]; !ok {
		return a, mkerror("httpauth: defaultRole missing")
	}
	return a, nil
}

// ProviderName returns the cloud provider ID.
func (a Authorizer) ProviderName() string {
	return ProviderName
}

// Login logs a user in. They will be redirected to dest or to the last
// location an authorization redirect was triggered (if found) on success. A
// message will be added to the session on failure with the reason.

func (a Authorizer) Login(rw http.ResponseWriter, req *http.Request, u string, p string) error {
	session, _ := a.cookiejar.Get(req, "auth")
	if session.Values["username"] != nil {
		return mkerror("already authenticated")
	}
	if user, err := a.backend.User(u); err == nil {
		verify := bcrypt.CompareHashAndPassword(user.Hash, []byte(p))
		if verify != nil {
			return mkerror("password doesn't match")
		}
	} else {
		return mkerror("user not found")
	}
	session.Values["username"] = u
	session.Save(req, rw)

	return nil
}

// Register and save a new user. Returns an error and adds a message if the
// username is in use.
//
// Pass in a instance of UserData with at least a username and email specified. If no role
// is given, the default one is used.

func (a Authorizer) AddUser(user authprovider.User, password string) error {
	if user.Username == "" {
		return mkerror("no username given")
	}
	if user.Email == "" {
		return mkerror("no email given")
	}
	if user.Hash != nil {
		return mkerror("hash will be overwritten")
	}
	if password == "" {
		return mkerror("no password given")
	}

	// Validate username
	_, err := a.backend.User(user.Username)
	if err == nil {
		return mkerror("user already exists")
	} else if err != ErrMissingUser {
		if err != nil {
			return mkerror(err.Error())
		}
		return nil
	}

	// Generate and save hash
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return mkerror("couldn't save password: " + err.Error())
	}
	user.Hash = hash

	// Validate role
	if user.Role == "" {
		user.Role = a.defaultRole
	} else {
		if _, ok := a.roles[user.Role]; !ok {
			return mkerror("non-existant role")
		}
	}

	err = a.backend.SaveUser(user)
	if err != nil {
		return mkerror(err.Error())
	}
	return nil
}

// Update changes data for an existing user. Needs thought...
func (a Authorizer) UpdateUser(req *http.Request, p string, e string) error {
	var (
		hash  []byte
		email string
	)
	authSession, err := a.cookiejar.Get(req, "auth")
	username, ok := authSession.Values["username"].(string)
	if !ok {
		return mkerror("not logged in")
	}
	user, err := a.backend.User(username)
	if err == ErrMissingUser {
		return mkerror("user doesn't exists")
	} else if err != nil {
		return mkerror(err.Error())
	}
	if p != "" {
		hash, err = bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
		if err != nil {
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

	newuser := authprovider.User{Username: username, Email: email, Hash: hash, Role: user.Role}

	err = a.backend.SaveUser(newuser)
	if err != nil {
		//a.addMessage(rw, req, err.Error())
	}
	return nil
}

// Authorize checks if a user is logged in and returns an error on failed
// authentication. If redirectWithMessage is set, the page being authorized
// will be saved and a "Login to do that." message will be saved to the
// messages list. The next time the user logs in, they will be redirected back
// to the saved page.
func (a Authorizer) Authorize(rw http.ResponseWriter, req *http.Request) error {
	authSession, err := a.cookiejar.Get(req, "auth")
	if err != nil {
		return mkerror("new authorization session")
	}
	if authSession.IsNew {
		return mkerror("no session existed")
	}
	username := authSession.Values["username"]
	if !authSession.IsNew && username != nil {
		_, err := a.backend.User(username.(string))
		if err == ErrMissingUser {
			authSession.Options.MaxAge = -1 // kill the cookie
			authSession.Save(req, rw)
			return mkerror("user not found")
		} else if err != nil {
			return mkerror(err.Error())
		}
	}
	if username == nil {
		return mkerror("user not logged in")
	}
	return nil
}

// AuthorizeRole runs Authorize on a user, then makes sure their role is at
// least as high as the specified one, failing if not.
func (a Authorizer) AuthorizeRole(rw http.ResponseWriter, req *http.Request, role string) error {
	r, ok := a.roles[role]
	if !ok {
		return mkerror("role not found")
	}
	if err := a.Authorize(rw, req); err != nil {
		return mkerror(err.Error())
	}
	authSession, _ := a.cookiejar.Get(req, "auth") // should I check err? I've already checked in call to Authorize
	username := authSession.Values["username"]
	if user, err := a.backend.User(username.(string)); err == nil {
		if a.roles[user.Role] >= r {
			return nil
		}
		return mkerror("user doesn't have high enough role")
	}
	return mkerror("user not found")
}

// CurrentUser returns the currently logged in user and a boolean validating
// the information.
func (a Authorizer) CurrentUser(rw http.ResponseWriter, req *http.Request) (user authprovider.User, e error) {
	if err := a.Authorize(rw, req); err != nil {
		return user, mkerror(err.Error())
	}
	authSession, _ := a.cookiejar.Get(req, "auth")

	username, ok := authSession.Values["username"].(string)
	if !ok {
		return user, mkerror("User not found in authsession")
	}
	return a.backend.User(username)
}

// Logout clears an authentication session and add a logged out message.
func (a Authorizer) Logout(rw http.ResponseWriter, req *http.Request) error {
	session, _ := a.cookiejar.Get(req, "auth")
	defer session.Save(req, rw)

	session.Options.MaxAge = -1 // kill the cookie
	return nil
}

// DeleteUser removes a user from the Authorize. ErrMissingUser is returned if
// the user to be deleted isn't found.
func (a Authorizer) DeleteUser(username string) error {
	err := a.backend.DeleteUser(username)
	if err != nil && err != ErrDeleteNull {
		return mkerror(err.Error())
	}
	return err
}

// Messages fetches a list of saved messages. Use this to get a nice message to print to
// the user on a login page or registration page in case something happened
// (username taken, invalid credentials, successful logout, etc).
func (a Authorizer) Messages(rw http.ResponseWriter, req *http.Request) []string {
	session, _ := a.cookiejar.Get(req, "messages")
	flashes := session.Flashes()
	session.Save(req, rw)
	var messages []string
	for _, val := range flashes {
		messages = append(messages, val.(string))
	}
	return messages
}

func (a Authorizer) ListUsers() (users []authprovider.User, err error) {

	if users, err = a.backend.Users(); err != nil {
		//glog.Errorf("Unable get the listeHHttpError of Users", err)
	}
	return users, err
}
