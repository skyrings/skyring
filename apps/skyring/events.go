package skyring

import (
	"encoding/json"
	"gopkg.in/mgo.v2/bson"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/utils"
	"net/http"
	"github.com/golang/glog"
)

func getEvents(rw http.ResponseWriter, req *http.Request) {
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	collection := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.NODE_EVENT_COLLECTION)
	var events []models.NodeEventStructure


	err := collection.Find(bson.M{}).All(&events)
	if err != nil {
		glog.Errorf("Error getting record from DB:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	bytes, err := json.Marshal(events)
	if err != nil {
		glog.Errorf("Unable to marshal the list of events:%s", err)
		util.HandleHttpError(rw, err)
		return
	}
	rw.Write(bytes)
}
