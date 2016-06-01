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
	"github.com/skyrings/skyring-common/dbprovider/mongodb"
	"github.com/skyrings/skyring-common/models"
	mail_notifier "github.com/skyrings/skyring-common/notifier"
	"github.com/skyrings/skyring-common/tools/logger"
	"io/ioutil"
	"net/http"
)

// @Title GetMailNotifier
// @Description Retrieves the mail notofier details
// @Param node-id path string true "UUID of the node"
// @Param disk-id path string true "UUID of the disk"
// @Success 200 {object} models.MailNotifier
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/mailnotifier
// @router /api/v1/mailnotifier [get]
func (a *App) GetMailNotifier(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	notifier, err := GetDbProvider().MailNotifierInterface().MailNotifier(ctxt)
	if err != nil {
		logger.Get().Error("%s-Unable to read MailNotifier from DB: %s", ctxt, err)
		HandleHttpError(rw, err)
		return
	} else {
		// explicitly set passcode as blank
		notifier.Passcode = ""
		json.NewEncoder(rw).Encode(notifier)
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

// @Title AddMailNotifier
// @Description Adds a mail notifier in the system
// @Param mail-id          form  string true  "email id to be used as From in notification mail"
// @Param passcode         form  string true  "password for the email"
// @Param smtpserver       form  string true  "SMTP Server address"
// @Param port             form  int    true  "SMTP server port"
// @Param encryption       form  bool   true  "if encryption required"
// @Param mailnotification form  bool   true  "if email notification required"
// @Param subprefix        form  string false "subject prefix text"
// @Success 200 {object} string
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/mailnotifier
// @router /api/v1/mailnotifier [post]
func (a *App) AddMailNotifier(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	if req.Method == "POST" {
		_, err := GetDbProvider().MailNotifierInterface().MailNotifier(ctxt)
		if err != mongodb.ErrMissingNotifier {
			HandleHttpError(rw, errors.New("Mail Notifier Record Already exists"))
			if err := logAuditEvent(EventTypes["ADD_MAIL_NOTIFIER"],
				fmt.Sprintf("Failed to add mail notifier"),
				fmt.Sprintf("Failed to add mail notifier. Error: %v",
					fmt.Errorf("Mail notifier already exists")),
				nil,
				nil,
				models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
				nil,
				ctxt); err != nil {
				logger.Get().Error("%s- Unable to log add mail notifier event. Error: %v", ctxt, err)
			}
			return
		}
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["ADD_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to add mail notifier"),
			fmt.Sprintf("Failed to add mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add mail notifier event. Error: %v", ctxt, err)
		}
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("%s-Unable to Unmarshall the data:%s", ctxt, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["ADD_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to add mail notifier"),
			fmt.Sprintf("Failed to add mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add mail notifier event. Error: %v", ctxt, err)
		}
		return
	}
	notifier, err := GetDetails(m)
	if err != nil {
		logger.Get().Error("%s-Insufficient details for mail notifier: %v", ctxt, notifier)
		HandleHttpError(rw, errors.New("insufficient detail for mail notifier"))
		if err := logAuditEvent(EventTypes["ADD_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to add mail notifier"),
			fmt.Sprintf("Failed to add mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add mail notifier event. Error: %v", ctxt, err)
		}
		return
	}

	if notifier.MailId == "" || notifier.Passcode == "" || notifier.SmtpServer == "" || notifier.Port == 0 || notifier.Encryption == "" {
		logger.Get().Error("%s-Insufficient details for add mail notifier: %v", ctxt, notifier)
		HandleHttpError(rw, errors.New("insufficient detail for add mail notifier"))
		if err := logAuditEvent(EventTypes["ADD_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to add mail notifier"),
			fmt.Sprintf("Failed to add mail notifier. Error: %v",
				fmt.Errorf("Insufficient details for adding mail notifier")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add mail notifier event. Error: %v", ctxt, err)
		}
		return
	}

	err = mail_notifier.SetMailClient(notifier, ctxt)
	if err != nil {
		logger.Get().Error("%s-Error setting the Mail Client Error: %v", ctxt, err)
		HandleHttpError(rw, errors.New(fmt.Sprintf("Error setting the Mail Client Error: %v", err)))
		if err := logAuditEvent(EventTypes["ADD_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to add mail notifier"),
			fmt.Sprintf("Failed to add mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add mail notifier event. Error: %v", ctxt, err)
		}
		return
	}

	err = GetDbProvider().MailNotifierInterface().SaveMailNotifier(ctxt, notifier)
	if err != nil {
		logger.Get().Error("%s-Error Updating the mail notifier info for: %s Error: %v", ctxt, notifier.MailId, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["ADD_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to add mail notifier"),
			fmt.Sprintf("Failed to add mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add mail notifier event. Error: %v", ctxt, err)
		}
	} else {
		if err := logAuditEvent(EventTypes["ADD_MAIL_NOTIFIER"],
			fmt.Sprintf("Successfully added mail notifier"),
			fmt.Sprintf("Successfully added mail notifier"),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log add mail notifier event. Error: %v", ctxt, err)
		}
	}
	return
}

// @Title TestMailNotifier
// @Description Test verifies the mail notification
// @Param mail-id          form  string true  "email id to be used as From in notification mail"
// @Param passcode         form  string true  "password for the email"
// @Param smtpserver       form  string true  "SMTP Server address"
// @Param port             form  int    true  "SMTP server port"
// @Param encryption       form  bool   true  "if encryption required"
// @Param mailnotification form  bool   true  "if email notification required"
// @Param subprefix        form  string false "subject prefix text"
// @Param recipient        form  string true  "Recipient mail ids (comma separated)"
// @Success 200 {object} string
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1
// @router /api/v1/testmailnotifier [post]
func (a *App) TestMailNotifier(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	var recepient []string
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["TEST_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to test mail notifier"),
			fmt.Sprintf("Failed to test mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log test mail notifier event. Error: %v", ctxt, err)
		}
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("%s-Unable to Unmarshall the data:%s", ctxt, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["TEST_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to test mail notifier"),
			fmt.Sprintf("Failed to test mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log test mail notifier event. Error: %v", ctxt, err)
		}
		return
	}
	notifier, err := GetDetails(m)
	if err != nil {
		logger.Get().Error("%s-Insufficient details for mail notifier: %v", ctxt, notifier)
		HandleHttpError(rw, errors.New("insufficient detail for mail notifier"))
		if err := logAuditEvent(EventTypes["TEST_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to test mail notifier"),
			fmt.Sprintf("Failed to test mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log test mail notifier event. Error: %v", ctxt, err)
		}
		return
	}
	if val, ok := m["recipient"]; ok {
		recepient = []string{val.(string)}
	}
	if notifier.MailId == "" || notifier.Passcode == "" || notifier.SmtpServer == "" || notifier.Port == 0 || notifier.Encryption == "" || len(recepient) == 0 {
		logger.Get().Error("%s-Insufficient details for Test mail notifier: %v", ctxt, notifier)
		HandleHttpError(rw, errors.New("insufficient details for Test mail notifier"))
		if err := logAuditEvent(EventTypes["TEST_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to test mail notifier"),
			fmt.Sprintf("Failed to test mail notifier. Error: %v",
				fmt.Errorf("Insufficient Details for testing mail notifier")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log test mail notifier event. Error: %v", ctxt, err)
		}
		return
	}

	if err := mail_notifier.TestMailNotify(notifier,
		"Test mail from skyring",
		"This is a test mail sent from skyring server",
		recepient, ctxt); err != nil {
		logger.Get().Error("%s-Unable test mail notifier. Error: %v", ctxt, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["TEST_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to test mail notifier"),
			fmt.Sprintf("Failed to test mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log test mail notifier event. Error: %v", ctxt, err)
		}
	} else {
		if err := logAuditEvent(EventTypes["TEST_MAIL_NOTIFIER"],
			fmt.Sprintf("Mail notifier tested successfully"),
			fmt.Sprintf("Mail notifier tested successfully"),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log test mail notifier event. Error: %v", ctxt, err)
		}
	}
	return
}

// @Title PatchMailNotifier
// @Description Partially update the mail notofoer details
// @Param mailnotification form  bool   false  "if email notification required"
// @Param subprefix        form  string false  "subject prefix text"
// @Success 200 {object} string
// @Failure 500 {object} string
// @Failure 400 {object} string
// @Resource /api/v1/mailnotifier
// @router /api/v1/mailnotifier [patch]
func (a *App) PatchMailNotifier(rw http.ResponseWriter, req *http.Request) {
	ctxt, err := GetContext(req)
	if err != nil {
		logger.Get().Error("Error Getting the context. error: %v", err)
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		logger.Get().Error("%s-Error parsing http request body:%s", ctxt, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["UPDATE_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to update mail notifier"),
			fmt.Sprintf("Failed to update mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update mail notifier event. Error: %v", ctxt, err)
		}
		return
	}
	var m map[string]interface{}

	if err = json.Unmarshal(body, &m); err != nil {
		logger.Get().Error("%s-Unable to Unmarshall the data:%s", ctxt, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["UPDATE_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to update mail notifier"),
			fmt.Sprintf("Failed to update mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update mail notifier event. Error: %v", ctxt, err)
		}
		return
	}

	notifier, err := GetDbProvider().MailNotifierInterface().MailNotifier(ctxt)
	if err != nil {
		logger.Get().Error("%s-MailNotifier not set: %s", ctxt, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["UPDATE_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to update mail notifier"),
			fmt.Sprintf("Failed to update mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update mail notifier event. Error: %v", ctxt, err)
		}
		return
	}
	parameterPresent := false
	if val, mn := m["mailnotification"]; mn {
		notifier.MailNotification = val.(bool)
		parameterPresent = true
	}
	if val, sp := m["subprefix"]; sp {
		parameterPresent = true
		notifier.SubPrefix = val.(string)
	}
	if !parameterPresent {
		logger.Get().Error("%s-Insufficient details for patching mail notifier: %v", ctxt, notifier)
		HandleHttpError(rw, errors.New("insufficient detail for mail notifier"))
		if err := logAuditEvent(EventTypes["UPDATE_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to update mail notifier"),
			fmt.Sprintf("Failed to update mail notifier. Error: %v",
				fmt.Errorf("Insufficent details to update mail notifier")),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update mail notifier event. Error: %v", ctxt, err)
		}
		return
	}
	err = GetDbProvider().MailNotifierInterface().SaveMailNotifier(ctxt, notifier)
	if err != nil {
		logger.Get().Error("%s-Error Updating the mail notifier info for: %s Error: %v", ctxt, notifier.MailId, err)
		HandleHttpError(rw, err)
		if err := logAuditEvent(EventTypes["UPDATE_MAIL_NOTIFIER"],
			fmt.Sprintf("Failed to update mail notifier"),
			fmt.Sprintf("Failed to update mail notifier. Error: %v",
				err),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update mail notifier event. Error: %v", ctxt, err)
		}
	} else {
		if err := logAuditEvent(EventTypes["UPDATE_MAIL_NOTIFIER"],
			fmt.Sprintf("Successfully updated mail notifier"),
			fmt.Sprintf("Successfully updated mail notifier"),
			nil,
			nil,
			models.NOTIFICATION_ENTITY_MAIL_NOTIFIER,
			nil,
			ctxt); err != nil {
			logger.Get().Error("%s- Unable to log update mail notifier event. Error: %v", ctxt, err)
		}
	}
	return
}
