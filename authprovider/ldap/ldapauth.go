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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/mqu/openldap"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/dao"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/authprovider"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/mgo.v2/bson"
	"io"
	"net/http"
	"strings"
)

const ProviderName = "ldapauthprovider"
const CipherKey = "Skyring - RedHat"

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
	directory    models.Directory
	defaultGroup string
	defaultRole  string
	roles        map[string]Role
}

func Authenticate(directory models.Directory, url string, user string, passwd string) error {
	if len(directory.Base) == 0 || len(passwd) == 0 {
		logger.Get().Error("Failed to find any LDAP configuration")
		return mkerror("Failed to find any LDAP configuration")
	}

	ldap, err := openldap.Initialize(url)

	if err != nil {
		logger.Get().Error("Failed to connect the server. error: %v", err)
		return err
	}
	ldap.SetOption(openldap.LDAP_OPT_PROTOCOL_VERSION, openldap.LDAP_VERSION3)
	if directory.Uid != "" {
		err = ldap.Bind(fmt.Sprintf("%s=%s,%s", directory.Uid, user, directory.Base), passwd)

		if err != nil {
			logger.Get().Error("Error binding to LDAP Server:%s. error: %v", url, err)
			return err
		}
	} else {
		if ldap.Bind(fmt.Sprintf("uid=%s,%s", user, directory.Base), passwd) != nil {
			err = ldap.Bind(fmt.Sprintf("cn=%s,%s", user, directory.Base), passwd)
			if err != nil {
				logger.Get().Error("Error binding to LDAP Server:%s. error: %v", url, err)
				return err
			}
		}
	}
	return nil
}

func GetUrl(ldapserver string, port uint) string {
	return fmt.Sprintf("ldap://%s:%d/", ldapserver, port)
}

func LdapAuth(a Authorizer, user, passwd string) bool {
	directory, err := a.GetDirectory()
	if err != nil {
		return false
	}
	url := GetUrl(directory.LdapServer, directory.Port)
	// Authenticating user
	err = Authenticate(directory, url, user, passwd)
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

	userDao := skyring.GetDbProvider().UserInterface()

	session := db.GetDatastore()
	c := session.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_LDAP)
	ldapDao := models.Directory{}
	err := c.Find(bson.M{}).One(&ldapDao)
	if err != nil {
		logger.Get().Info("Failed to open ldap db collection:%s", err)
	}
	//Create the Provider
	if provider, err := NewAuthorizer(userDao, ldapDao); err != nil {
		logger.Get().Error("Unable to initialize the authorizer for Ldapauthprovider. error: %v", err)
		panic(err)
	} else {
		return &provider, nil
	}

}

func NewAuthorizer(userDao dao.UserInterface, ldapDao models.Directory) (Authorizer, error) {
	var a Authorizer
	a.userDao = userDao
	a.directory.LdapServer = ldapDao.LdapServer
	a.directory.Port = ldapDao.Port
	a.directory.Base = ldapDao.Base
	a.directory.DomainAdmin = ldapDao.DomainAdmin
	a.directory.Password = ldapDao.Password
	a.directory.Uid = ldapDao.Uid
	a.directory.FirstName = ldapDao.FirstName
	a.directory.DisplayName = ldapDao.DisplayName
	a.directory.LastName = ldapDao.LastName
	a.directory.Email = ldapDao.Email
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
	if !session.IsNew {
		if session.Values["username"] == u {
			logger.Get().Info("User %s already logged in", u)
			return nil
		} else {
			return mkerror("user " + session.Values["username"].(string) + " is already logged in")
		}
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
	// Update the new username in session before persisting to DB
	session.Values["username"] = u
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
func (a Authorizer) ListExternalUsers(search string, page, count int) (externalUsers models.ExternalUsers, err error) {
	directory, err := a.GetDirectory()
	if err != nil {
		return externalUsers, err
	}
	url := GetUrl(directory.LdapServer, directory.Port)
	Uid := "Uid"
	DisplayName := "DisplayName"
	FirstName := "CN"
	LastName := "SN"
	Email := "mail"
	if directory.Uid != "" {
		Uid = directory.Uid
	}
	if directory.DisplayName != "" {
		DisplayName = directory.DisplayName
	}
	if directory.FirstName != "" {
		FirstName = directory.FirstName
	}
	if directory.LastName != "" {
		LastName = directory.LastName
	}
	if directory.Email != "" {
		Email = directory.Email
	}

	ldap, err := openldap.Initialize(url)
	if err != nil {
		logger.Get().Error("failed to connect the LDAP/AD server. error: %v", err)
		return externalUsers, err
	}

	if directory.DomainAdmin != "" {
		block, err := aes.NewCipher([]byte(CipherKey))
		if err != nil {
			logger.Get().Error("failed to generate new cipher")
			return externalUsers, nil
		}

		ciphertext := []byte(directory.Password)
		iv := ciphertext[:aes.BlockSize]
		stream := cipher.NewOFB(block, iv)
		hkey := make([]byte, 100)
		stream = cipher.NewOFB(block, iv)
		stream.XORKeyStream(hkey, ciphertext[aes.BlockSize:])
		err = ldap.Bind(fmt.Sprintf("%s=%s,%s", Uid, directory.DomainAdmin, directory.Base), string(hkey))
		if err != nil {
			logger.Get().Error("Error binding to LDAP Server:%s. error: %v", url, err)
			return externalUsers, err
		}
	}

	scope := openldap.LDAP_SCOPE_SUBTREE
	// If the search string is empty, it will list all the users
	// If the search string contains 'mail=tjey*' it will returns the list of all
	// users start with 'tjey'
	// If the search string contains 'tim' this will return list of all users
	// names contains the word 'tim'
	// Possible search strings 'mail=t*redhat.com' / 'tim*' / '*john*' / '*peter'
	filter := "(objectclass=*)"
	if len(search) > 0 {
		if strings.Contains(search, "=") {
			filter = fmt.Sprintf("(%s*)", search)
		} else if strings.Contains(search, "*") {
			filter = fmt.Sprintf("(cn=%s)", search)
		} else {
			filter = fmt.Sprintf("(cn=*%s*)", search)
		}
	}

	attributes := []string{Uid, DisplayName, FirstName, LastName, Email}
	rv, err := ldap.SearchAll(directory.Base, scope, filter, attributes)

	if err != nil {
		logger.Get().Error("Failed to search LDAP/AD server. error: %v", err)
		return externalUsers, err
	}

	from := (page-1)*count + 1
	to := from + count - 1
	i := 0

	for _, entry := range rv.Entries() {
		i++
		if i < from {
			continue
		}
		if i > to {
			break
		}
		user := models.User{}
		for _, attr := range entry.Attributes() {
			switch attr.Name() {
			case Uid:
				user.Username = strings.Join(attr.Values(), ", ")
			case Email:
				user.Email = strings.Join(attr.Values(), ", ")
			case FirstName:
				user.FirstName = strings.Join(attr.Values(), ", ")
			case LastName:
				user.LastName = strings.Join(attr.Values(), ", ")
			}
		}
		// Assiging the default roles
		user.Role = a.defaultRole
		user.Groups = append(user.Groups, a.defaultGroup)
		user.Type = authprovider.External
		if len(user.Username) != 0 {
			externalUsers.Users = append(externalUsers.Users, user)
		}
	}
	externalUsers.TotalCount = rv.Count()
	externalUsers.StartIndex = from
	externalUsers.EndIndex = to
	if externalUsers.EndIndex > externalUsers.TotalCount {
		externalUsers.EndIndex = externalUsers.TotalCount
	}
	return externalUsers, nil
}

// List the users in DB
func (a Authorizer) ListUsers() (users []models.User, err error) {
	if users, err = a.userDao.Users(nil); err != nil {
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
func (a Authorizer) UpdateUser(username string, m map[string]interface{}, u string) error {
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
		} else {
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

	if val, ok := m["status"]; ok {
		s := val.(bool)
		user.Status = s
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

func (a Authorizer) GetDirectory() (directory models.Directory, err error) {
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

func (a Authorizer) SetDirectory(directory models.Directory) error {
	if directory.LdapServer == "" {
		logger.Get().Error("no directory server name provided")
		return mkerror("no directory server name provided")
	}
	if directory.Port == 0 {
		logger.Get().Error("no directory server port number provided")
		return mkerror("no directory server port number provided")
	}
	if directory.Base == "" {
		logger.Get().Error("no directory server connection string provided")
		return mkerror("no directory server connection string provided")
	}

	session := db.GetDatastore()
	c := session.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_LDAP)
	val, err := c.Count()
	if err != nil {
		logger.Get().Error("failed to get ldap records count")
		return mkerror("failed to get ldap records count")
	}
	if val > 0 {
		c.RemoveAll(nil)
	}

	pswd := []byte(directory.Password)
	block, err := aes.NewCipher([]byte(CipherKey))
	if err != nil {
		logger.Get().Info("Failed to generate cipher:%s", err)
		return mkerror("Failed to generate cipher")
	}
	chkey := make([]byte, aes.BlockSize+len(pswd))
	iv := chkey[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		logger.Get().Info("Failed to configure ldap:%s", err)
		return mkerror("Failed to configure ldap")
	}
	stream := cipher.NewOFB(block, iv)
	stream.XORKeyStream(chkey[aes.BlockSize:], pswd)

	err = c.Insert(&models.Directory{directory.LdapServer, directory.Port, directory.Base,
		directory.DomainAdmin, string(chkey), directory.Uid,
		directory.FirstName, directory.LastName, directory.DisplayName, directory.Email})

	return nil
}
