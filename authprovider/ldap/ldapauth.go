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
	"github.com/goincremental/negroni-sessions"
	"github.com/golang/glog"
	"github.com/mqu/openldap"
	"github.com/skyrings/skyring/authprovider"
	"github.com/skyrings/skyring/models"
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
	backend          authprovider.AuthBackend
	ldapServer       string
	port             int
	connectionString string
	defaultRole      string
	roles            map[string]Role
}

type LdapProviderCfg struct {
	LdapServer Directory `json:"ldapserver"`
	UserRoles  RolesConf `joson:"userroles"`
}

type RolesConf struct {
	Roles       map[string]Role
	DefaultRole string
}

type Directory struct {
	Address string
	Port    int
	Base    string
}

func Authenticate(url string, base string,
	user string, passwd string) error {

	ldap, err := openldap.Initialize(url)
	defer ldap.Close()

	if err != nil {
		glog.Errorf("Failed to connect the server!")
		return err
	}

	ldap.SetOption(openldap.LDAP_OPT_PROTOCOL_VERSION, openldap.LDAP_VERSION3)
	userConnStr := fmt.Sprintf("uid=%s,%s", user, base)

	err = ldap.Bind(userConnStr, passwd)
	if err != nil {
		glog.Errorf("Error binding to LDAP Server:%v", err)
		return err
	}
	return nil
}

func GetUrl(ldapserver string, port int) string {
	return fmt.Sprintf("ldap://%s:%d/", ldapserver, port)
}

func LdapAuth(a Authorizer, user, passwd string) bool {
	url := GetUrl(a.ldapServer, a.port)
	glog.V(3).Infof("URL VALUE IS:%s", url)
	// Authenticating user
	err := Authenticate(url, a.connectionString, user, passwd)
	if err != nil {
		glog.Errorf("Authentication failed: %s", err)
		return false
	} else {
		glog.Infof("Ldap user login success!")
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
		glog.Errorln(errStr)
		return nil, fmt.Errorf(errStr)
	}

	providerCfg := LdapProviderCfg{}

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
		glog.Errorf("Unable to initialize the DB backend for Ldapauthprovider:%s", err)
		panic(err)
	}
	//Create the Provider
	if provider, err := NewAuthorizer(backend,
		providerCfg.LdapServer.Address,
		providerCfg.LdapServer.Port,
		providerCfg.LdapServer.Base,
		providerCfg.UserRoles.DefaultRole,
		providerCfg.UserRoles.Roles); err != nil {
		glog.Errorf("Unable to initialize the authorizer for Ldapauthprovider:%s", err)
		panic(err)
	} else {
		return &provider, nil
	}

}

func NewAuthorizer(backend authprovider.AuthBackend, address string, port int, base string,
	defaultRole string, roles map[string]Role) (Authorizer, error) {
	var a Authorizer
	a.backend = backend
	a.ldapServer = address
	a.port = port
	a.connectionString = base
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
		glog.Infoln("Already logged in")
		return nil
	}
	errStrNotAllowed := "This user is not allowed. Status Disabled"
	// Verify user allowed to user usm with group privilage in the db
	if user, err := a.backend.User(u); err == nil {
		errStr := "Passwords Doesnot match"
		if user.Status {
			if user.Type == authprovider.External {
				if LdapAuth(a, u, p) {
					glog.Infof("Login Success for LDAP")
				} else {
					glog.Errorln(errStr)
					return mkerror(errStr)
				}
			} else {
				verify := bcrypt.CompareHashAndPassword(user.Hash, []byte(p))
				if verify != nil {
					glog.Errorln(errStr)
					return mkerror(errStr)
				}
			}
		} else {
			glog.Errorln(errStrNotAllowed)
			return mkerror(errStrNotAllowed)
		}
	} else {
		glog.Errorln(errStrNotAllowed)
		return mkerror(errStrNotAllowed)
	}
	session.Set("username", u)

	return nil
}

// Logout clears an authentication session and add a logged out message.
func (a Authorizer) Logout(rw http.ResponseWriter, req *http.Request) error {
	session := sessions.GetSession(req)
	session.Delete("username")
	return nil
}

// List the LDAP users
func (a Authorizer) ListExternalUsers() (users []models.User, err error) {

	url := GetUrl(a.ldapServer, a.port)

	ldap, err := openldap.Initialize(url)
	if err != nil {
		glog.Errorf("failed to connect the LDAP/AD server:%s", err)
		return nil, err
	}

	scope := openldap.LDAP_SCOPE_SUBTREE

	filter := "(objectclass=*)"
	attributes := []string{"Uid", "UidNumber", "CN", "SN",
		"Givenname", "Displayname", "mail"}

	rv, err := ldap.SearchAll(a.connectionString, scope, filter, attributes)

	if err != nil {
		glog.Errorf("failed to search LDAP/AD server:%s", err)
		return nil, err
	}

	for _, entry := range rv.Entries() {
		user := models.User{}
		for _, attr := range entry.Attributes() {
			switch attr.Name() {
			case "Uid":
				user.Username = strings.Join(attr.Values(), ", ")
			case "Mail":
				user.Email = strings.Join(attr.Values(), ", ")
			default:
				glog.Errorf("This property is not supported:%s", attr.Name())
			}
		}
		// find the user role and group from the db and assign it
		users = append(users, user)

	}
	return users, nil
}

// List the users in DB
func (a Authorizer) ListUsers() (users []models.User, err error) {

	if users, err = a.backend.Users(); err != nil {
		glog.Errorf("Unable get the list of Users: %v", err)
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
		glog.Errorln("no user name given")
		return mkerror("no username given")
	}
	if user.Email == "" {
		glog.Errorln("no email given")
		return mkerror("no email given")
	}

	//Set the usertype to external
	user.Type = authprovider.External
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

	user.Hash = nil

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

	if user.Type == authprovider.Internal {
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
	} else {
		return mkerror("Operation Not Supported")
	}

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
func (a Authorizer) GetUser(u string) (user models.User, e error) {

	// This should search and fetch the user name based on the given value
	// and can also check whether the available users are already imported
	// into the database or not.
	user, e = a.backend.User(u)
	if e != nil {
		glog.Errorf("Error retrieving the user:%s", e)
		return user, e
	}
	return user, nil
}

// DeleteUser removes a user from the Authorize. ErrMissingUser is returned if
// the user to be deleted isn't found.
// This will delete the ldap user name from the db so that
// he will be no longer available for login to use skyring
func (a Authorizer) DeleteUser(username string) error {
	err := a.backend.DeleteUser(username)
	if err != nil {
		glog.Errorf("Unable delete the user: %s", err)
		return err
	}
	return nil
}
