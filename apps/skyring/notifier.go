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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/skyrings/skyring-common/dbprovider/mongodb"
	"github.com/skyrings/skyring-common/models"
	mail_notifier "github.com/skyrings/skyring-common/notifier"
	"github.com/skyrings/skyring-common/tools/logger"
	"io/ioutil"
	"net/http"
)

func (a *App) GetMailNotifier(rw http.ResponseWriter, req *http.Request) {
	notifier, err := GetDbProvider().MailNotifierInterface().MailNotifier()
	if err != nil {
		logger.Get().Error("Unable to read MailNotifier from DB: %s", err)
		HandleHttpError(rw, err)
		return
	} else {
		n := map[string]interface{}{
			"mailid":     notifier.MailId,
			"smtpserver": notifier.SmtpServer,
			"port":       notifier.Port,
			"skipverify": notifier.SkipVerify,
			"encryption": notifier.Encryption,
		}
		json.NewEncoder(rw).Encode(n)
	}
}

func GetDetails(m map[string]interface{}) (models.MailNotifier, error) {

	var notifier models.MailNotifier
	notifier.SkipVerify = true

	if val, ok := m["mailid"]; ok {
		notifier.MailId = val.(string)
	}
	if val, ok := m["password"]; ok {
		notifier.Passcode = base64.StdEncoding.EncodeToString([]byte(val.(string)))
	}
	if val, ok := m["smtpserver"]; ok {
		notifier.SmtpServer = val.(string)
	}
	if val, ok := m["port"]; ok {
		notifier.Port = int(val.(float64))
	}
	if val, ok := m["skipverify"]; ok {
		notifier.SkipVerify = val.(bool)
	}
	if val, ok := m["encryption"]; ok {
		notifier.Encryption = val.(string)
	}
	if val, ok := m["mailnotification"]; ok {
		notifier.MailNotification = val.(bool)
	} else {
		err = errors.New("insufficient detail for mail notifier")
		return notifier, err
	}
	if val, ok := m["subprefix"]; ok {
		notifier.SubPrefix = val.(string)
	}
	return notifier, nil
}

func (a *App) AddMailNotifier(rw http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		_, err := GetDbProvider().MailNotifierInterface().MailNotifier()
		if err != mongodb.ErrMissingNotifier {
			HandleHttpError(rw, errors.New("Mail Notifier Record Already exists"))
			return
		}
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("Error parsing http request body:%s", err)
		HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		HandleHttpError(rw, err)
		return
	}
	notifier, err := GetDetails(m)
	if err != nil {
		logger.Get().Error("Insufficient details for mail notifier: %v", notifier)
		HandleHttpError(rw, errors.New("insufficient detail for mail notifier"))
		return
	}

	if notifier.MailId == "" || notifier.Passcode == "" || notifier.SmtpServer == "" || notifier.Port == 0 || notifier.Encryption == "" {
		logger.Get().Error("Insufficient details for add mail notifier: %v", notifier)
		HandleHttpError(rw, errors.New("insufficient detail for add mail notifier"))
		return
	}

	err = mail_notifier.SetMailClient(notifier)
	if err != nil {
		logger.Get().Error("Error setting the Mail Client Error: %v", err)
		HandleHttpError(rw, errors.New(fmt.Sprintf("Error setting the Mail Client Error: %v", err)))
		return
	}

	err = GetDbProvider().MailNotifierInterface().SaveMailNotifier(notifier)
	if err != nil {
		logger.Get().Error("Error Updating the mail notifier info for: %s Error: %v", notifier.MailId, err)
		HandleHttpError(rw, err)
	}
	return
}
func (a *App) TestMailNotifier(rw http.ResponseWriter, req *http.Request) {
	var recepient []string
	vars := mux.Vars(req)
	recepient = []string{vars["recipient"]}
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("Error parsing http request body:%s", err)
		HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		HandleHttpError(rw, err)
		return
	}
	notifier, err := GetDetails(m)
	if err != nil {
		logger.Get().Error("Insufficient details for mail notifier: %v", notifier)
		HandleHttpError(rw, errors.New("insufficient detail for mail notifier"))
		return
	}

	if notifier.MailId == "" || notifier.Passcode == "" || notifier.SmtpServer == "" || notifier.Port == 0 || notifier.Encryption == "" {
		logger.Get().Error("Insufficient details for Test mail notifier: %v", notifier)
		HandleHttpError(rw, errors.New("insufficient details for Test mail notifier"))
		return
	}

	mail_notifier.TestMailNotify(notifier, "Test mail from skyring", "This is a test mail sent from skyring server", recepient)
	return
}
