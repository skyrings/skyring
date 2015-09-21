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
	"encoding/json"
	"errors"
	"github.com/codegangsta/negroni"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/utils"
	"io/ioutil"
	"net/http"
)

func (a *App) AddDefaultUser() error {

	defaultUser := model.User{Username: "admin", Email: "admin@localhost", Role: "admin"}

	if err := a.authProvider.AddUser(defaultUser, "admin"); err != nil {
		glog.Errorf("Unable to create default User:%s", err)
		return err
	}

	return nil
}

func (a *App) SetAuthRoutes(router *mux.Router, authReqdRouter *mux.Router) error {
	//Login doesnot require authentication
	router.
		Methods("POST").
		Path("/login").
		Name("login").
		Handler(http.HandlerFunc(a.loginHandler))

	//All other routes needs to be authenticated
	authReqdRouter.
		Methods("POST").
		Path("/logout").
		Name("logout").
		Handler(http.HandlerFunc(a.logoutHandler))
	authReqdRouter.
		Methods("GET").
		Path("/users").
		Name("getUsers").
		Handler(http.HandlerFunc(a.getUsersHandler))
	authReqdRouter.
		Methods("POST").
		Path("/users").
		Name("addUsers").
		Handler(http.HandlerFunc(a.addUsersHandler))
	authReqdRouter.
		Methods("DELETE").
		Path("/users").
		Name("deleteUsers").
		Handler(http.HandlerFunc(a.deleteUsersHandler))

	// Create a new negroni for the loginrequired middleware
	router.PathPrefix("/logout").Handler(negroni.New(
		negroni.HandlerFunc(a.LoginRequired),
		negroni.Wrap(authReqdRouter),
	))
	router.PathPrefix("/users").Handler(negroni.New(
		negroni.HandlerFunc(a.LoginRequired),
		negroni.Wrap(authReqdRouter),
	))

	return nil
}

func (a *App) loginHandler(rw http.ResponseWriter, req *http.Request) {

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		glog.Errorf("Error parsing http request body:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(body, &m); err != nil {
		glog.Errorf("Unable to Unmarshall the data:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	if err := a.authProvider.Login(rw, req, m["username"].(string), m["password"].(string)); err != nil {
		glog.Errorf("Unable to login User:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	bytes, _ := json.Marshal(`{'message': 'Logged in'}`)
	rw.Write(bytes)
}

func (a *App) logoutHandler(rw http.ResponseWriter, req *http.Request) {
	if err := a.authProvider.Logout(rw, req); err != nil {
		glog.Errorf("Unable to logout User:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	bytes, _ := json.Marshal(`{'message': 'Logged out'}`)
	rw.Write(bytes)
}

func (a *App) getUsersHandler(rw http.ResponseWriter, req *http.Request) {

	users, err := a.authProvider.ListUsers()
	if err != nil {
		glog.Errorf("Unable to List the users:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	bytes, err := json.Marshal(users)
	if err != nil {
		glog.Errorf("Unable marshal the list of Users:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	rw.Write(bytes)

}
func (a *App) addUsersHandler(rw http.ResponseWriter, req *http.Request) {

	var user model.User

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		glog.Errorf("Error parsing http request body:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(body, &m); err != nil {
		glog.Errorf("Unable to Unmarshall the data:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}

	user.Username = m["username"].(string)
	user.Email = m["email"].(string)
	user.Role = m["role"].(string)

	if err := a.authProvider.AddUser(user, m["password"].(string)); err != nil {
		glog.Errorf("Unable to create User:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
}

func (a *App) deleteUsersHandler(rw http.ResponseWriter, req *http.Request) {

	var user model.User

	//Get the request details from requestbody

	if err := parseAuthRequestBody(req, &user); err != nil {
		glog.Errorf("Unable to parse the request data:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
	if err := a.authProvider.DeleteUser(user.Username); err != nil {
		glog.Errorf("Unable to delete User:%s", err)
		util.HandleHHttpError(rw, err)
		return
	}
}

func parseAuthRequestBody(req *http.Request, user *model.User) error {
	//Get the request details from requestbody
	var (
		body []byte
		err  error
	)
	if body, err = ioutil.ReadAll(req.Body); err != nil {
		glog.Errorf("Error parsing http request body:%s", err)
		return errors.New(err.Error())
	}
	if err = json.Unmarshal(body, &user); err != nil {
		glog.Errorf("Unable to Unmarshall the data:%s", err)
		return errors.New(err.Error())
	}
	return nil
}
