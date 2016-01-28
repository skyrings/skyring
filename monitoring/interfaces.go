package monitoring

type MonitoringManagerInterface interface {
	QueryDB(params map[string]interface{}) (interface{}, error)
	PushToDb(metrics map[string]map[string]string) error
}
