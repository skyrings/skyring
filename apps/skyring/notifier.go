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
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	"github.com/skyrings/skyring-common/models"
	mail_notifier "github.com/skyrings/skyring-common/notifier"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/utils"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"net/http"
)

func (a *App) GetMailNotifier(rw http.ResponseWriter, req *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_MAIL_NOTIFIER)
	var notifier models.MailNotifier
	if err := collection.Find(nil).One(&notifier); err != nil {
		logger.Get().Error("Unable to read MailNotifier from DB: %s", err)
		util.HandleHttpError(rw, err)
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

func (a *App) AddMailNotifier(rw http.ResponseWriter, req *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_MAIL_NOTIFIER)
	var notifier models.MailNotifier

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("Error parsing http request body:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("Unable to Unmarshall the data:%s", err)
		util.HandleHttpError(rw, err)
		return
	}

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

	if notifier.MailId == "" || notifier.Passcode == "" || notifier.SmtpServer == "" || notifier.Port == 0 || notifier.Encryption == "" {
		logger.Get().Error("Insufficient details of add mail notifier: %v", notifier)
		util.HandleHttpError(rw, errors.New("insufficient detail for add mail notifier"))
		return
	}

	err = mail_notifier.SetMailClient(notifier)
	if err != nil {
		logger.Get().Error("Error setting the Mail Client Error: %v", err)
		util.HandleHttpError(rw, errors.New(fmt.Sprintf("Error setting the Mail Client Error: %v", err)))
		return
	}
	_, err = collection.Upsert(bson.M{}, bson.M{"$set": notifier})
	if err != nil {
		logger.Get().Error("Error Updating the mail notifier info for: %s Error: %v", notifier.MailId, err)
		util.HandleHttpError(rw, err)
	}
	return
}
