package authprovider

import (
	"encoding/json"
	"errors"
	"github.com/skyrings/skyring/models"
	"io/ioutil"
	"net/http"
)

var Err bool

type MockAuthorizer struct{}

func MockInitAuthProvider() AuthInterface {
	var a MockAuthorizer
	return a
}

func (test MockAuthorizer) Login(rw http.ResponseWriter, req *http.Request, username string, password string) error {
	if username == "" {
		return errors.New("error")
	}
	if password == "" {
		return errors.New("error")
	}
	return nil
}

func (test MockAuthorizer) Logout(rw http.ResponseWriter, req *http.Request) error {
	var msg map[string]bool
	body, _ := ioutil.ReadAll(req.Body)
	if _ = json.Unmarshal(body, &msg); msg["logout"] == false {
		return errors.New("error")
	}
	return nil
}

func (test MockAuthorizer) Authorize(rw http.ResponseWriter, req *http.Request) error {
	return nil
}

func (test MockAuthorizer) AuthorizeRole(rw http.ResponseWriter, req *http.Request, role string) error {
	return nil
}

func (test MockAuthorizer) AddUser(user models.User, password string) error {
	if user.Username == "" {
		return errors.New("error")
	}
	if password == "" {
		return errors.New("error")
	}
	return nil
}

func (test MockAuthorizer) UpdateUser(username string, m map[string]interface{}) error {
	if username == "" {
		return errors.New("error")
	}
	if username != m["username"] {
		return errors.New("error")
	}
	if m["username"] == "" {
		return errors.New("error")
	}
	return nil
}

func (test MockAuthorizer) GetUser(username string, req *http.Request) (models.User, error) {
	var user models.User
	if username != "" {
		//Assigning Dummy values for models
		str := `{
				"username"				:"admin",
				"email"					: "test@redhat.com",
				"role"					:"software tester",
				"groups"				:["abc","123"],
				"type"					:1,
				"status"				:true,
				"firstname"				:"skyring",
				"lastname"				:"uss",
				"notificationenabled"	:true
			}`
		var jsontrr = []byte(str)
		json.Unmarshal(jsontrr, &user)
		return user, nil
	}
	return user, errors.New("error")
}

func (test MockAuthorizer) ListUsers() ([]models.User, error) {
	var user = []models.User{}
	if Err == true {
		//Assigning Dummy values for models
		var str = []string{
			`{
				"username"				:"admin",
				"email"					: "test@redhat.com",
				"role"					:"software tester",
				"groups"				:["abc","123"],
				"type"					:1,
				"status"				:true,
				"firstname"				:"skyring",
				"lastname"				:"uss",
				"notificationenabled"	:true
			}`,
			`{
				"username"				:"tester",
				"email"					: "test100@redhat.com",
				"role"					:"abc",
				"groups"				:["xyz","321"],
				"type"					:2,
				"status"				:false,
				"firstname"				:"skyring",
				"lastname"				:"uss",
				"notificationenabled"	:true
			}`,
		}
		user = make([]models.User, len(user)+2)
		for i := range str {
			var jsontrr = []byte(str[i])
			json.Unmarshal(jsontrr, &user[i])
		}
		return user, nil
	}
	return user, errors.New("error")
}

func (test MockAuthorizer) DeleteUser(username string) error {
	if username == "" {
		return errors.New("error")
	}
	return nil
}

func (test MockAuthorizer) ListExternalUsers() ([]models.User, error) {
	var user = []models.User{}
	if Err == true {
		//Assigning Dummy values for models
		var str = []string{
			`{
				"username"				:"admin",
				"email"					: "test@redhat.com",
				"role"					:"software tester",
				"groups"				:["abc","123"],
				"type"					:1,
				"status"				:true,
				"firstname"				:"skyring",
				"lastname"				:"uss",
				"notificationenabled"	:true
			}`,
			`{
				"username"				:"tester",
				"email"					: "test100@redhat.com",
				"role":"abc","groups"	:["xyz","321"],
				"type":2,"status"		:false,
				"firstname"				:"skyring",
				"lastname"				:"uss",
				"notificationenabled"	:true
			}`,
		}
		user = make([]models.User, len(user)+2)
		for i := range str {
			var jsontrr = []byte(str[i])
			json.Unmarshal(jsontrr, &user[i])
		}
		return user, nil
	}
	return user, errors.New("error")
}
