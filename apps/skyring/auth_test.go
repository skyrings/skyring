package skyring

import (
	"bytes"
	"github.com/skyrings/skyring/authprovider"
	"github.com/skyrings/skyring/models"
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
	if err := AddDefaultUser(); err != nil {
		t.Error("")
	}
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
	if rw.Code != 200 {
		t.Error()
	}
	//Checking Unmarshal Condition with invalid input
	jsonStr = []byte(`
	{
    	"username": ",
    	"password": test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.login(rw, req)
	if rw.Code <= 400 {
		t.Error()
	}
	//Checking Unmarshal Condition with nil input
	jsonStr = []byte(nil)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.login(rw, req)
	if rw.Code <= 400 {
		t.Error()
	}
	//Checking login Condition invalid username
	jsonStr = []byte(`
	{
    	"username": "",
    	"password": "test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.login(rw, req)
	if rw.Code <= 400 {
		t.Error()
	}
	//Checking login Condition invalid password
	jsonStr = []byte(`
	{
    	"username": "test",
    	"password": ""
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.login(rw, req)
	if rw.Code <= 400 {
		t.Error()
	}
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
	if rw.Code <= 400 {
		t.Error()
	}
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
	if rw.Code >= 400 {
		t.Error()
	}
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
	if rw.Code >= 400 {
		t.Error()
	}
	rw = httptest.NewRecorder()
	//Falg for make error
	authprovider.Err = false
	app.getUsers(rw, req)
	//Check whether values are assigned correctly or no
	if rw.Code == 200 {
		t.Error()
	}

}

func Test_getUser(t *testing.T) {

	url := "http://test"
	var req *http.Request
	req, _ = http.NewRequest("GET", url, nil)
	rw := httptest.NewRecorder()
	//Mock CallVars Function
	Callvars = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	//Checking getUser without Error
	app.getUser(rw, req)
	if rw.Code >= 400 {
		t.Error()
	}
	//Checking getUser with Error
	Callvars = func(req *http.Request) map[string]string {
		return (map[string]string{"username": ""}) //without username
	}
	req, _ = http.NewRequest("GET", url, nil)
	rw = httptest.NewRecorder()
	app.getUser(rw, req)
	//Check whether the error is generated or not
	if rw.Code == 200 {
		t.Error()
	}
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
	if rw.Code >= 400 {
		t.Error()
	}
	rw = httptest.NewRecorder()
	//Falg for make error
	authprovider.Err = false
	app.getExternalUsers(rw, req)
	//Check whether values are assigned correctly or no
	if rw.Code == 200 {
		t.Error()
	}

}

func Test_addUsers(t *testing.T) {

	url := "http://test"
	var req *http.Request
	var jsonStr = []byte(nil)
	//Initializing Http Request
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	//Initializing ResponseWriter
	rw := httptest.NewRecorder()
	//Test with invalid data
	app.addUsers(rw, req)
	if rw.Code == 200 {
		t.Error()
	}
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
	if rw.Code == 200 {
		t.Error()
	}
	//nil data for check unmarshal condition
	jsonStr = []byte(nil)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	app.addUsers(rw, req)
	if rw.Code == 200 {
		t.Error()
	}
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
	if rw.Code == 200 {
		t.Error()
	}
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
	if rw.Code != 200 {
		t.Error()
	}
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
	//Mock CallVars Function
	Callvars = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	app.modifyUsers(rw, req)
	if rw.Code == 200 {
		t.Error()
	}
	//Invalid input for unmorshall
	jsonStr = []byte(nil)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	//Mock CallVars Function
	Callvars = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	app.modifyUsers(rw, req)
	if rw.Code == 200 {
		t.Error()
	}
	//Invalid input for update user condition,because usernames are not same
	jsonStr = []byte(`
	{
    	"username": "test",
    	"password": "test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	//Mock CallVars Function
	Callvars = func(req *http.Request) map[string]string {
		return (map[string]string{"username": ""})
	}
	app.modifyUsers(rw, req)
	if rw.Code == 200 {
		t.Error()
	}
	//Invalid input for update user condition,because usernames are not same
	jsonStr = []byte(`
	{
    	"username": "",
    	"password": "test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	//Mock CallVars Function
	Callvars = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	app.modifyUsers(rw, req)
	if rw.Code == 200 {
		t.Error()
	}
	//Valid input for update user condition
	jsonStr = []byte(`
	{
    	"username": "admin",
    	"password": "test"
	}`)
	req, _ = http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	rw = httptest.NewRecorder()
	//Mock CallVars Function
	Callvars = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	//Checking getUser without Error
	app.modifyUsers(rw, req)
	if rw.Code >= 400 {
		t.Error()
	}
}

func Test_deleteUser(t *testing.T) {
	url := "http://test"
	var req *http.Request
	req, _ = http.NewRequest("POST", url, nil)
	rw := httptest.NewRecorder()
	//Mock CallVars Function
	Callvars = func(req *http.Request) map[string]string {
		return (map[string]string{"username": "admin"})
	}
	//Checking DeleteUsrer condition without any Error
	app.deleteUser(rw, req)
	if rw.Code >= 400 {
		t.Error()
	}
	req, _ = http.NewRequest("POST", url, nil)
	rw = httptest.NewRecorder()
	//Mock CallVars Function
	Callvars = func(req *http.Request) map[string]string {
		return (map[string]string{"username": ""})
	}
	//Checking DeleteUser condition without Error
	app.deleteUser(rw, req)
	if rw.Code == 400 {
		t.Error()
	}
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
	if err := parseAuthRequestBody(req, user); err == nil {
		t.Error()
	}
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
	if err := parseAuthRequestBody(req, user); err != nil {
		t.Error()
	}
}
