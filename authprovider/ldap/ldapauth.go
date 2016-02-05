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
package ldapauthprovider

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mqu/openldap"
	"github.com/skyrings/skyring-common/dao"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/authprovider"
	"golang.org/x/crypto/bcrypt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

const ProviderName = "ldapauthprovider"

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
	userDao      dao.UserInterface
	directory    Directory
	defaultGroup string
	defaultRole  string
	roles        map[string]Role
}

type LdapProviderCfg struct {
	LdapServer Directory  `json:"ldapserver"`
	UserRoles  RolesConf  `json:"userroles"`
	UserGroups GroupsConf `json:"usergroups"`
}

type GroupsConf struct {
	DefaultGroup string
}

type RolesConf struct {
	Roles       map[string]Role
	DefaultRole string
}

type Directory struct {
	Address     string
	Port        int
	Base        string
	DomainAdmin string
	Password    string
	Uid         string
	FullName    string
	DisplayName string
}

func Authenticate(a Authorizer, url string, user string, passwd string) error {
	ldap, err := openldap.Initialize(url)

	if err != nil {
		logger.Get().Error("Failed to connect the server. error: %v", err)
		return err
	}

	ldap.SetOption(openldap.LDAP_OPT_PROTOCOL_VERSION, openldap.LDAP_VERSION3)
	if a.directory.Uid != "" {
		err = ldap.Bind(fmt.Sprintf("%s=%s,%s", a.directory.Uid, user, a.directory.Base), passwd)

		if err != nil {
			logger.Get().Error("Error binding to LDAP Server:%s. error: %v", url, err)
			return err
		}
	} else {
		if ldap.Bind(fmt.Sprintf("uid=%s,%s", user, a.directory.Base), passwd) != nil {
			err = ldap.Bind(fmt.Sprintf("cn=%s,%s", user, a.directory.Base), passwd)
			if err != nil {
				logger.Get().Error("Error binding to LDAP Server:%s. error: %v", url, err)
				return err
			}
		}
	}
	return nil
}

func GetUrl(ldapserver string, port int) string {
	return fmt.Sprintf("ldap://%s:%d/", ldapserver, port)
}

func LdapAuth(a Authorizer, user, passwd string) bool {
	url := GetUrl(a.directory.Address, a.directory.Port)
	// Authenticating user
	err := Authenticate(a, url, user, passwd)
	if err != nil {
		logger.Get().Error("Authentication failed for user: %s. error: %v", user, err)
		return false
	} else {
		logger.Get().Info("Ldap user login success for user: %s", user)
		return true
	}
}

func init() {
	authprovider.RegisterAuthProvider(ProviderName, func(config io.Reader) (authprovider.AuthInterface, error) {
		return NewLdapAuthProvider(config)
	})
}

func mkerror(msg string) error {
	return errors.New(msg)
}

func NewLdapAuthProvider(config io.Reader) (*Authorizer, error) {
	if config == nil {
		errStr := "missing configuration file for Ldap Auth provider"
		logger.Get().Error(errStr)
		return nil, fmt.Errorf(errStr)
	}

	providerCfg := LdapProviderCfg{}

	bytes, err := ioutil.ReadAll(config)
	if err != nil {
		logger.Get().Error("Error reading Configuration file. error: %v", err)
		return nil, err
	}
	if err = json.Unmarshal(bytes, &providerCfg); err != nil {
		logger.Get().Error("Unable to Unmarshall the data. error: %v", err)
		return nil, err
	}
	userDao := skyring.GetDbProvider().UserInterface()
	//Create the Provider
	if provider, err := NewAuthorizer(userDao, providerCfg); err != nil {
		logger.Get().Error("Unable to initialize the authorizer for Ldapauthprovider. error: %v", err)
		panic(err)
	} else {
		return &provider, nil
	}

}

func NewAuthorizer(userDao dao.UserInterface, providerCfg LdapProviderCfg) (Authorizer, error) {
	var a Authorizer
	a.userDao = userDao
	a.directory.Address = providerCfg.LdapServer.Address
	a.directory.Port = providerCfg.LdapServer.Port
	a.directory.Base = providerCfg.LdapServer.Base
	a.directory.DomainAdmin = providerCfg.LdapServer.DomainAdmin
	a.directory.Password = providerCfg.LdapServer.Password
	a.directory.Uid = providerCfg.LdapServer.Uid
	a.directory.FullName = providerCfg.LdapServer.FullName
	a.directory.DisplayName = providerCfg.LdapServer.DisplayName
	a.roles = providerCfg.UserRoles.Roles
	a.defaultRole = providerCfg.UserRoles.DefaultRole
	a.defaultGroup = providerCfg.UserGroups.DefaultGroup
	if _, ok := a.roles[a.defaultRole]; !ok {
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
		logger.Get().Error("Error Getting the session. error: %v", err)
		return err
	}
	if session.IsNew {
		session.Values["username"] = u
	} else {
		logger.Get().Info("User %s already logged in", u)
		return nil
	}
	errStrNotAllowed := "This user is not allowed. Status Disabled"
	// Verify user allowed to user usm with group privilage in the db
	if user, err := a.userDao.User(u); err == nil {
		errStr := fmt.Sprintf("Password does not match for user: %s", u)
		if user.Status {
			if user.Type == authprovider.External {
				if LdapAuth(a, u, p) {
					logger.Get().Info("Login Success for LDAP")
				} else {
					logger.Get().Error(errStr)
					return mkerror(errStr)
				}
			} else {
				verify := bcrypt.CompareHashAndPassword(user.Hash, []byte(p))
				if verify != nil {
					logger.Get().Error(errStr)
					return mkerror(errStr)
				}
			}
		} else {
			logger.Get().Error(errStrNotAllowed)
			return mkerror(errStrNotAllowed)
		}
	} else {
		logger.Get().Error("User does not exist: %s", user)
		return mkerror("User does not exist")
	}
	if err = session.Save(req, rw); err != nil {
		logger.Get().Error("Error saving the session for user: %s. error: %v", u, err)
		return err
	}

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

// List the LDAP users
func (a Authorizer) ListExternalUsers() (users []models.User, err error) {
	url := GetUrl(a.directory.Address, a.directory.Port)
	Uid := "Uid"
	DisplayName := "DisplayName"
	FullName := "CN"
	if a.directory.Uid != "" {
		Uid = a.directory.Uid
	}
	if a.directory.DisplayName != "" {
		DisplayName = a.directory.DisplayName
	}
	if a.directory.FullName != "" {
		FullName = a.directory.FullName
	}

	ldap, err := openldap.Initialize(url)
	if err != nil {
		logger.Get().Error("failed to connect the LDAP/AD server. error: %v", err)
		return nil, err
	}

	if a.directory.DomainAdmin != "" {
		err = ldap.Bind(fmt.Sprintf("%s=%s,%s", Uid, a.directory.DomainAdmin, a.directory.Base), a.directory.Password)
		if err != nil {
			logger.Get().Error("Error binding to LDAP Server:%s. error: %v", url, err)
			return nil, err
		}
	}

	scope := openldap.LDAP_SCOPE_SUBTREE
	filter := "(objectclass=*)"
	attributes := []string{Uid, DisplayName, FullName, "Mail"}

	rv, err := ldap.SearchAll(a.directory.Base, scope, filter, attributes)

	if err != nil {
		logger.Get().Error("Failed to search LDAP/AD server. error: %v", err)
		return nil, err
	}

	for _, entry := range rv.Entries() {
		user := models.User{}
		fullName := ""
		for _, attr := range entry.Attributes() {
			switch attr.Name() {
			case Uid:
				user.Username = strings.Join(attr.Values(), ", ")
			case "Mail":
				user.Email = strings.Join(attr.Values(), ", ")
			case DisplayName:
				user.FirstName = strings.Join(attr.Values(), ", ")
			case FullName:
				fullName = strings.Join(attr.Values(), ", ")
			}
			if len(fullName) != 0 && len(user.FirstName) != 0 {
				lastName := strings.Split(fullName, user.FirstName)
				if len(lastName) > 1 {
					user.LastName = strings.TrimSpace(lastName[1])
				}
			}

		}
		// Assiging the default roles
		user.Role = a.defaultRole
		user.Groups = append(user.Groups, a.defaultGroup)
		user.Type = authprovider.External
		if len(user.Username) != 0 {
			users = append(users, user)
		}
	}
	return users, nil
}

// List the users in DB
func (a Authorizer) ListUsers() (users []models.User, err error) {
	if users, err = a.userDao.Users(); err != nil {
		logger.Get().Error("Unable get the list of users. error: %v", err)
		return users, err
	}
	return users, nil
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

	user.Status = true

	// Validate username
	_, err := a.userDao.User(user.Username)
	if err == nil {
		logger.Get().Error("User %s already exists", user.Username)
		return mkerror("user already exists")
	} else if err.Error() != ErrMissingUser.Error() {
		if err != nil {
			logger.Get().Error("Error retrieving details of user: %s. error: %v", user.Username, err)
			return mkerror(err.Error())
		}
		return nil
	}
	// Validate role
	if user.Role == "" {
		user.Role = a.defaultRole
	} else {
		if _, ok := a.roles[user.Role]; !ok {
			logger.Get().Error("Non Existing Role: %s", user.Role)
			return mkerror("non-existant role")
		}
	}
	user.Hash = nil
	if user.Type == authprovider.Internal {

		if password == "" {
			logger.Get().Error("no password given for user: %s", user.Username)
			return mkerror("no password given")
		}
		// Generate and save hash
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			logger.Get().Error("couldn't save password for user: %s. error: %v", user.Username, err)
			return mkerror("couldn't save password: " + err.Error())
		}
		user.Hash = hash

	}
	err = a.userDao.SaveUser(user)
	if err != nil {
		logger.Get().Error("Error saving the user: %s. error: %v", user.Username, err)
		return mkerror(err.Error())
	}
	return nil
}

// Update changes data for an existing user. Needs thought...
//Just added for completeness. Will revisit later
func (a Authorizer) UpdateUser(username string, m map[string]interface{}) error {
	var (
		hash    []byte
		updated bool
	)

	user, err := a.userDao.User(username)
	if err != nil {
		logger.Get().Error("Error retrieving the user: %s. error: %v", username, err)
		return err
	}

	if user.Type == authprovider.Internal {
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
					updated = true
				}
			}
		}
	}
	if val, ok := m["email"]; ok {
		e := val.(string)
		user.Email = e
		updated = true
	}

	if val, ok := m["notificationenabled"]; ok {
		n := val.(bool)
		user.NotificationEnabled = n
	}

	if updated {
		err = a.userDao.SaveUser(user)
		if err != nil {
			logger.Get().Error("Error saving details for the user: %s to DB. error: %v", username, err)
			return err
		}
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

// CurrentUser returns the currently logged in user and a boolean validating
// the information.
func (a Authorizer) GetUser(u string, req *http.Request) (user models.User, e error) {
	// This should search and fetch the user name based on the given value
	// and can also check whether the available users are already imported
	// into the database or not.

	//if username is me, get the currently loggedin user
	if u == authprovider.CurrentUser {
		session, err := skyring.Store.Get(req, "session-key")
		if err != nil {
			logger.Get().Error("Error getting the session for user: %s. error: %v", u, err)
			return user, err
		}
		if val, ok := session.Values["username"]; ok {
			u = val.(string)
		} else {
			logger.Get().Error("Unable to identify the user: %s from session", u)
			return user, mkerror("Unable to identify the user from session")
		}
	}
	user, e = a.userDao.User(u)
	if e != nil {
		logger.Get().Error("Error retrieving the user: %s. error: %v", user, e)
		return user, e
	}
	return user, nil
}

// DeleteUser removes a user from the Authorize. ErrMissingUser is returned if
// the user to be deleted isn't found.
// This will delete the ldap user name from the db so that
// he will be no longer available for login to use skyring
func (a Authorizer) DeleteUser(username string) error {
	err := a.userDao.DeleteUser(username)
	if err != nil {
		logger.Get().Error("Unable to delete the user: %s. error: %v", username, err)
		return err
	}
	return nil
}
