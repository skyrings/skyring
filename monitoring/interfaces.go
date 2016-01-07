package monitoring


type MonitoringManagerInterface interface {
	QueryDB(resource, time string) (interface{}, error)
}
