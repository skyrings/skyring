package monitoring

type MonitoringManagerInterface interface {
	QueryDB(params map[string]interface{}) (interface{}, error)
	PushToDb(interface{}) error
}
