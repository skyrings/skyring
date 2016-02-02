package skyring

import (
	"bytes"
	"github.com/skyrings/skyring/authprovider"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/testassert"
	"github.com/skyrings/skyring/tools/logger"
	"net/http"
	"net/http/httptest"
	"testing"
)

var app = &App{}

func Test_AddDefaultUser(t *testing.T) {

	//Mock logger for avoiding Invalid address Error during Testing
	logger.MockInit()
	//Mock Authprovider.AuthInterface to test
	AuthProviderInstance = authprovider.MockInitAuthProvider()
	//Calling actual funtion and test it has throw non nil err value
	testassert.AssertNotEqual(t, AddDefaultUser(), nil, "Unable to create default User")
}

func MockGetAuthProvider(test authprovider.AuthInterface) {
	AuthProviderInstance = test
}

func Test_login(t *testing.T) {

	url := "http://test"
	var req *http.Request
	var jsonStr = []byte(`
	{
    	"username": "test",
    	"password": "test"
	}`)
	//Initializing Http Request
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	//Initializing ResponseWriter
	rw := httptest.NewRecorder()
	app.login(rw, req)
	testassert.AssertNotEqual(t, rw.Code, 200, "Unable to login")
	//Checking Unmarshal Condition with invalid input
	jsonStr = []byte(`
	{
    	"username": ",
    	"password": test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.login(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Json unmorshall error not handled correctly")
	//Checking Unmarshal Condition with nil input
	jsonStr = []byte(nil)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.login(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Json unmorshall accept nil value")
	//Checking login Condition invalid username
	jsonStr = []byte(`
	{
    	"username": "",
    	"password": "test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.login(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Invalid username")
	//Checking login Condition invalid password
	jsonStr = []byte(`
	{
    	"username": "test",
    	"password": ""
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.login(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Invalid password")
}

func Test_logout(t *testing.T) {

	//Check logout is unsuccess
	url := "http://test"
	var req *http.Request
	var jsonStr = []byte(`
	{
    	"logout": false
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw := httptest.NewRecorder()
	app.logout(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Logout failed")
	//Check logout is success
	jsonStr = []byte(`
	{
    	"logout": true
	}`)
	//Initializing Http Request
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	//Initializing ResponseWriter
	rw = httptest.NewRecorder()
	app.logout(rw, req)
	testassert.AssertNotEqual(t, rw.Code, 200, "Error condition not checked")
}

func Test_getUsers(t *testing.T) {

	url := "http://test"
	var req *http.Request
	req, _ = http.NewRequest("GET", url, nil)
	rw := httptest.NewRecorder()
	//Falg for avoid error
	authprovider.Err = true
	app.getUsers(rw, req)
	//Check whether values are assigned correctly or no
	testassert.AssertNotEqual(t, rw.Code, 200, "Unable to List the users")
	rw = httptest.NewRecorder()
	//Falg for make error
	authprovider.Err = false
	app.getUsers(rw, req)
	//Check whether values are assigned correctly or no
	testassert.AssertEqual(t, rw.Code, 200, "Error condition not checked")

}

func Test_getUser(t *testing.T) {

	url := "http://test"
	var req *http.Request
	req, _ = http.NewRequest("GET", url, nil)
	rw := httptest.NewRecorder()
	//Mock RequestParams Function
	RequestParams = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	//Checking getUser without Error
	app.getUser(rw, req)
	testassert.AssertNotEqual(t, rw.Code, 200, "Unable to get perticular user")
	//Checking getUser with Error
	RequestParams = func(req *http.Request) map[string]string {
		return (map[string]string{"username": ""}) //without username
	}
	req, _ = http.NewRequest("GET", url, nil)
	rw = httptest.NewRecorder()
	app.getUser(rw, req)
	//Check whether the error is generated or not
	testassert.AssertEqual(t, rw.Code, 200, "Error is not generated correctly")
}

func Test_getExternalUsers(t *testing.T) {

	url := "http://test"
	var req *http.Request
	req, _ = http.NewRequest("GET", url, nil)
	rw := httptest.NewRecorder()
	//Falg for avoid error
	authprovider.Err = true
	app.getExternalUsers(rw, req)
	//Check whether values are assigned correctly or no
	testassert.AssertNotEqual(t, rw.Code, 200, "Unable to get externalUsers")
	rw = httptest.NewRecorder()
	//Falg for make error
	authprovider.Err = false
	app.getExternalUsers(rw, req)
	//Check whether values are assigned correctly or no
	testassert.AssertEqual(t, rw.Code, 200, "Error conditon not checked")

}

func Test_addUsers(t *testing.T) {

	url := "http://test"
	var req *http.Request
	//nil data for check unmarshal condition
	var jsonStr = []byte(nil)
	//Initializing Http Request
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	//Initializing ResponseWriter
	rw := httptest.NewRecorder()
	//Test with invalid data
	app.addUsers(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Condition for check Json unmorshall error is not checked")
	//Error data for check unmarshal condition
	jsonStr = []byte(`
	{
    	"username"				:"admi
    	"password"				:"abc
    	"email"					:"test@Redhat.com",
    	"role"					:"tester",
    	"type"					1,
    	"firstname"				:"xyz",
    	"lastname"				:"abc",
    	"notificationenabled"	:true
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	//Test with invalid data
	app.addUsers(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Condition for check Json unmorshall error is not checked")
	//Error data for check Adduser condition
	jsonStr = []byte(`
	{
    	"username"				:"",
    	"password"				:"abc",
    	"email"					:"test@Redhat.com",
    	"role"					:"tester",
    	"type"					:1,
    	"firstname"				:"xyz",
    	"lastname"				:"abc",
    	"notificationenabled"	:true
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.addUsers(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Invalid data is assigned but not checked")
	//Checking Adduser condition with valid data
	jsonStr = []byte(`
	{
    	"username"				:"xyz",
    	"password"				:"abc",
    	"email"					:"test@Redhat.com",
    	"role"					:"tester",
    	"type"					:1,
    	"firstname"				:"xyz",
    	"lastname"				:"abc",
    	"notificationenabled"	:true
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.addUsers(rw, req)
	testassert.AssertNotEqual(t, rw.Code, 200, "Vaild data but Error occured")
}
func Test_modifyUsers(t *testing.T) {
	url := "http://test"
	//Invalid input for unmorshall
	var jsonStr = []byte(`
	{
    	"username" ""
    	"password": "test
	}`)
	var req *http.Request
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw := httptest.NewRecorder()
	//Mock RequestParams Function
	RequestParams = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	app.modifyUsers(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Updation done with invalid data")
	//Invalid input for unmorshall
	jsonStr = []byte(nil)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	//Mock RequestParams Function
	RequestParams = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	app.modifyUsers(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Authorization not work")
	//Invalid input for update user condition,because usernames are not same
	jsonStr = []byte(`
	{
    	"username": "test",
    	"password": "test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	//Mock RequestParams Function
	RequestParams = func(req *http.Request) map[string]string {
		return (map[string]string{"username": ""})
	}
	app.modifyUsers(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Authorization not work")
	//Invalid input for update user condition,because usernames are not same
	jsonStr = []byte(`
	{
    	"username": "",
    	"password": "test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	//Mock RequestParams Function
	RequestParams = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	app.modifyUsers(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Authorization not work")
	//Valid input for update user condition
	jsonStr = []byte(`
	{
    	"username": "admin",
    	"password": "test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	//Mock RequestParams Function
	RequestParams = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	//Checking getUser without Error
	app.modifyUsers(rw, req)
	testassert.AssertNotEqual(t, rw.Code, 200, "Valid data but Updation in unsuccess")
}

func Test_deleteUser(t *testing.T) {
	url := "http://test"
	var req *http.Request
	req, _ = http.NewRequest("POST", url, nil)
	rw := httptest.NewRecorder()
	//Mock RequestParams Function
	RequestParams = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	//Checking DeleteUsrer condition without any Error
	app.deleteUser(rw, req)
	testassert.AssertNotEqual(t, rw.Code, 200, "Valid data but deletion is unsuccess")
	req, _ = http.NewRequest("POST", url, nil)
	rw = httptest.NewRecorder()
	//Mock RequestParams Function
	RequestParams = func(req *http.Request) map[string]string {
		return (map[string]string{"username": ""})
	}
	//Checking DeleteUser condition without Error
	app.deleteUser(rw, req)
	testassert.AssertEqual(t, rw.Code, 200, "Invalid deletion")
}

func Test_parseAuthRequestBody(t *testing.T) {
	url := "http://test"
	var user *models.User
	//Invalid input for unmorshall
	var jsonStr = []byte(`
	{
    	"username" ""
    	"password": "test"
	}`)
	var req *http.Request
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	testassert.AssertEqual(t, parseAuthRequestBody(req, user), nil, "Error parsing http request body")
	//nil input for unmorshall
	jsonStr = []byte(nil)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err := parseAuthRequestBody(req, user); err == nil {
		t.Error()
	}
	//valid input for unmorshall
	jsonStr = []byte(`
	{
    	"username" :"test",
    	"password": "test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	testassert.AssertNotEqual(t, parseAuthRequestBody(req, user), nil, "Valid http request but error generated")

}
